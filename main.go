// Command mcp-gdocs serves Google Slides, Sheets, Docs and Drive to an MCP client as
// one signed-in person.
//
// The slides tools are the reason it exists. A general-purpose Google server edits a
// deck by moving text boxes and inventing styling, and the result is a deck that looks
// broken to everyone who opens it. The tools here work the way the editor does: native
// nested lists, styles copied off the template's own slides, real tables, and a
// thumbnail to check the result with. There is deliberately no way to send an arbitrary
// batch of requests through this server, because that is the thing that produces
// crooked decks.
//
// Removal stops at the edge of a file: inside a presentation or a document it is ordinary
// editing, and the tools that do it live in their own groups, off unless --tools asks for
// them by name. Nothing here deletes a file, a folder or a drive.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	"github.com/svesh87/mcp-gdocs/internal/auth"
	"github.com/svesh87/mcp-gdocs/internal/config"
	"github.com/svesh87/mcp-gdocs/internal/google"
	"github.com/svesh87/mcp-gdocs/internal/tools"
	"github.com/svesh87/mcp-gdocs/internal/transport"
)

// version is set at build time. A build without it says so rather than claiming a number.
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mcp-gdocs: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	// The login command is separate because it needs a person: consent happens in a
	// browser, and nobody but the account owner can give it.
	if len(args) > 0 && args[0] == "login" {
		return login(args[1:])
	}

	flags, showVersion, err := parseFlags("mcp-gdocs", args)
	if err != nil || showVersion {
		return err
	}

	cfg, err := loadConfig(flags)
	if err != nil {
		return err
	}

	return serve(cfg)
}

// loadConfig validates the flags against the environment.
func loadConfig(flags config.Flags) (*config.Config, error) {
	return config.Load(flags, config.OSEnv)
}

// parseFlags reads the command line shared by the server and the login command.
func parseFlags(name string, args []string) (config.Flags, bool, error) {
	set := flag.NewFlagSet(name, flag.ContinueOnError)

	var (
		transportID = set.String("transport", config.TransportStdio,
			"transport: "+config.TransportStdio+" or "+config.TransportHTTP)
		address     = set.String("address", "127.0.0.1:8819", "address to listen on with "+config.TransportHTTP)
		credentials = set.String("credentials", "", "path to the OAuth client file from Google Cloud Console")
		tokenDir    = set.String("token-dir", "", "directory holding the token of the signed-in account")
		scopes      = set.String("scopes", "", "scopes to ask for, comma separated; empty means the default four")
		allowWrite  = set.Bool("allow-write", false, "register the tools that change documents (off by default)")
		toolGroups  = set.String("tools", "",
			"tool groups to offer, comma separated: slides-read, slides-write, slides-delete and the "+
				"same for sheets, docs and drive, plus drive-share; a family name means its read and "+
				"write halves; all means everything except removal and sharing. Empty means all. "+
				"Also read from "+config.EnvTools)
		filesDir = set.String("files-dir", "",
			"directory exports may be written to and imports read from; empty means no file access at all")
		showVersion = set.Bool("version", false, "print the version and exit")
	)

	if err := set.Parse(args); err != nil {
		return config.Flags{}, false, err
	}

	if *showVersion {
		fmt.Println("mcp-gdocs " + version)
		return config.Flags{}, true, nil
	}

	return config.Flags{
		Transport:   *transportID,
		Address:     *address,
		Credentials: *credentials,
		TokenDir:    *tokenDir,
		Scopes:      *scopes,
		AllowWrite:  *allowWrite,
		FilesDir:    *filesDir,
		Tools:       *toolGroups,
	}, false, nil
}

func serve(cfg *config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The set of tools is settled before anything else: a typo in --tools should be a
	// refusal on the first line of output, not after the credentials have been read.
	groups, err := tools.ParseGroups(cfg.Tools)
	if err != nil {
		return err
	}

	authenticator, err := newAuthenticator(cfg)
	if err != nil {
		return err
	}

	// The server starts whether or not anybody has signed in. That is deliberate: in a
	// container the sign-in happens through this very server's /login page, so refusing
	// to start without a token would make signing in impossible.
	provider := &lazyClients{authenticator: authenticator}

	mcpServer, err := newToolServer(cfg, provider, groups)
	if err != nil {
		return err
	}

	if cfg.Transport == config.TransportHTTP {
		// One process, several windows on it: /mcp is the whole set, and each family
		// has a path of its own so a project that needs one of them does not carry the
		// other ninety tool descriptions in its context.
		perPath := map[string]map[tools.Group]bool{transport.MCPPath: groups}
		for _, family := range tools.Families() {
			perPath[transport.MCPPath+"/"+family] = tools.Narrow(groups, family)
		}

		// And behind each window, a second server for connections that say they can cope
		// with a tool list that grows. It offers the same tools through the same
		// configuration — the ceiling is still --tools, and the window is still the
		// window — but a few at a time.
		servers := map[string]*server.MCPServer{transport.MCPPath: mcpServer}
		discovery := map[string]*server.MCPServer{}

		for path, allowed := range perPath {
			full := mcpServer
			if path != transport.MCPPath {
				// A family window keeps two of Drive's readings — finding a file by name
				// and asking what it is — because a bridge that reads a workbook from a
				// slides window is no use if the workbook cannot be found first. Only
				// where drive-read was allowed at all: this widens a window, not the
				// ceiling.
				var alsoOffer []string
				if groups[tools.DriveRead] {
					alsoOffer = tools.WindowDriveReads()
				}

				full, err = newToolServer(cfg, provider, allowed, alsoOffer...)
				if err != nil {
					return err
				}
				servers[path] = full
			}

			narrow, _ := tools.NarrowFrom(full, tools.DiscoveryGroups(allowed))
			discovery[path] = narrow
		}

		pages := auth.NewWebLogin(authenticator, cfg.AuthToken).Handlers()
		return transport.ServeHTTP(servers, discovery, cfg.Address, cfg.AuthToken, pages)
	}

	served := make(chan error, 1)
	go func() { served <- transport.ServeStdio(mcpServer) }()

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
		return nil
	}
}

// newToolServer builds one MCP server offering one set of tool groups. Several of them
// share everything else: the same client provider, the same token, the same sign-in.
func newToolServer(cfg *config.Config, provider tools.Clients, groups map[tools.Group]bool,
	alsoOffer ...string) (*server.MCPServer, error) {
	mcpServer := server.NewMCPServer("mcp-gdocs", version, server.WithToolCapabilities(true))

	if err := tools.Register(mcpServer, tools.Options{
		Clients:    provider,
		AllowWrite: cfg.AllowWrite,
		FilesDir:   cfg.FilesDir,
		Groups:     groups,
		AlsoOffer:  alsoOffer,
	}); err != nil {
		return nil, err
	}

	return mcpServer, nil
}

// login signs in from a machine that has a browser.
func login(args []string) error {
	flags, showVersion, err := parseFlags("mcp-gdocs login", args)
	if err != nil || showVersion {
		return err
	}

	// The login command needs no port of its own and no bearer token, so the transport
	// is forced to the one that demands neither.
	flags.Transport = config.TransportStdio

	cfg, err := loadConfig(flags)
	if err != nil {
		return err
	}

	authenticator, err := newAuthenticator(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return auth.LoginCLI(ctx, authenticator, os.Stdout)
}

func newAuthenticator(cfg *config.Config) (*auth.Authenticator, error) {
	credentials, err := clientCredentials(cfg)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.TokenDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating the token directory %s: %w", cfg.TokenDir, err)
	}

	return auth.New(credentials, auth.NewStore(cfg.TokenPath()), cfg.Scopes), nil
}

// clientCredentials takes the OAuth client from wherever the configuration says it is.
//
// The token is not part of this: it stays a file in the token directory, because it is
// written by the server itself every time the access token is refreshed, and a secret
// store is not somewhere a process writes to on its own.
func clientCredentials(cfg *config.Config) (auth.Credentials, error) {
	if cfg.Credentials == "" {
		return auth.Credentials{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			AuthURI:      auth.DefaultAuthURI,
			TokenURI:     auth.DefaultTokenURI,
		}, nil
	}

	return auth.LoadCredentials(cfg.Credentials)
}

// lazyClients builds the Google client the first time a tool needs one.
//
// Nothing is built at startup because there may be no token yet, and nothing is rebuilt
// afterwards because the client refreshes its own access token. A sign-in that happens
// while the server runs is picked up by the next call, since the failure to build is not
// remembered.
type lazyClients struct {
	authenticator *auth.Authenticator

	mu     sync.Mutex
	client *google.Client
}

// Google hands out the client, building it if this is the first call since a sign-in.
func (l *lazyClients) Google(ctx context.Context) (*google.Client, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.client != nil {
		return l.client, nil
	}

	httpClient, err := l.authenticator.HTTPClient(ctx)
	if err != nil {
		if errors.Is(err, auth.ErrNoToken) {
			return nil, err
		}
		return nil, fmt.Errorf("reaching Google: %w", err)
	}

	l.client = google.New(httpClient)

	return l.client, nil
}

// compile-time check that the provider satisfies what the tools expect.
var _ tools.Clients = (*lazyClients)(nil)

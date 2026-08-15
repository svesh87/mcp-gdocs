package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Discovery is the answer to a hundred and fifty tool descriptions arriving in an agent's
// context before it has called anything.
//
// It is switched on per connection rather than per server, and that is not a convenience: it
// only works with a client that re-reads its tool list when told to. Claude Code does — the
// notification is handled, the cached list dropped and tools/list fetched again. Codex, as far
// as anybody has been able to establish, does not: a list taken at connection time stays. So a
// server that always started narrow would be a server Codex cannot write with, and the choice
// has to belong to whoever is connecting.
//
// The mechanism is per-session tools. A connection in discovery mode gets a small set — the
// reference, this tool, and the readings, because every job starts by reading something — and
// what it asks for afterwards is added to that session alone. Another connection to the same
// path sees none of it.

// Catalogue is what discovery may hand out: every tool the configuration allows, minus the
// ones that must never arrive by asking.
type Catalogue struct {
	tools map[string]*server.ServerTool
}

// NewCatalogue takes the tools of a fully-registered server and keeps the ones discovery may
// offer.
//
// Removal is left out whatever --tools allowed. The groups that remove things are a decision
// an operator makes at startup, and a tool that arrives because an agent asked for it by name
// is not that decision — it is the agent's. The ceiling stays where it was put.
func NewCatalogue(full *server.MCPServer) *Catalogue {
	catalogue := &Catalogue{tools: map[string]*server.ServerTool{}}

	for name, tool := range full.ListTools() {
		group, err := GroupOf(name)
		if err != nil || strings.Contains(string(group), "delete") {
			continue
		}
		catalogue.tools[name] = tool
	}

	return catalogue
}

// Names is every tool the catalogue holds, in order.
func (c *Catalogue) Names() []string {
	names := make([]string, 0, len(c.tools))
	for name := range c.tools {
		names = append(names, name)
	}

	return sortedStrings(names)
}

// DiscoveryGroups is the set a connection starts with in discovery mode: the reference, the
// readings of whatever the configuration allowed, and this tool.
//
// The readings are in it on purpose. Every job here begins by reading something — a sample
// deck, a tab, a document — and an agent that has to ask for the right to read before it can
// look at anything spends a round trip learning what it already knew it needed.
func DiscoveryGroups(enabled map[Group]bool) map[Group]bool {
	narrow := map[Group]bool{Common: true}
	for group := range enabled {
		if strings.HasSuffix(string(group), "-read") {
			narrow[group] = true
		}
	}

	return narrow
}

// NarrowFrom builds the discovery twin of a server out of that server's own tools.
//
// Registering the whole set a second time would work and would be worse in two ways: it
// doubles the work at startup for every path, and it leaves two registries that can drift
// apart — a tool added to one and not the other is a difference nobody would see until an
// agent asked for something the catalogue promised. Taking the tools from the server that
// already has them makes that impossible: the two are the same objects.
func NarrowFrom(full *server.MCPServer, keep map[Group]bool) (*server.MCPServer, *Catalogue) {
	narrow := server.NewMCPServer("mcp-gdocs", "", server.WithToolCapabilities(true))

	var starting []server.ServerTool
	for name, tool := range full.ListTools() {
		group, err := GroupOf(name)
		if err != nil || !keep[group] {
			continue
		}
		starting = append(starting, *tool)
	}
	narrow.AddTools(starting...)

	catalogue := NewCatalogue(full)
	RegisterDiscovery(narrow, catalogue)

	return narrow, catalogue
}

// RegisterDiscovery adds the tool that hands the rest out.
//
// What the server already offers is remembered here, before this tool is added, so that a
// search never spends its answer on names the caller can already see. Half the readings have
// "list" in them, and a question about a nested list would otherwise come back as seven
// listing tools the session already had.
func RegisterDiscovery(srv *server.MCPServer, catalogue *Catalogue) {
	already := map[string]bool{}
	for name := range srv.ListTools() {
		already[name] = true
	}

	handler := &discovery{catalogue: catalogue, server: srv, already: already}

	srv.AddTool(mcp.NewTool("gdocs_find_tools",
		mcp.WithDescription("Ask for the tools you need. This connection started with the reading tools "+
			"and this one; everything else — creating, styling, filling, exporting — arrives when you "+
			"ask for it here, and stays for the rest of the session. Ask in the words of the job: "+
			"\"bullets\", \"picture as a slide background\", \"dropdown\", \"copy a tab into another "+
			"workbook\". The answer lists what was added, with what each one does, so the next call can "+
			"be the real one. Asking twice for the same thing is free. Removal never arrives this way: "+
			"it is switched on at startup or not at all."),
		mcp.WithString("about", mcp.Description(
			"What you are trying to do, in a few words. Matched against every tool's name and its "+
				"description.")),
		mcp.WithArray("names", mcp.WithStringItems(), mcp.Description(
			"Exact tool names, when you already know them — from a skill, or from an earlier answer.")),
		mcp.WithNumber("limit", mcp.DefaultNumber(8), mcp.Description(
			"How many to add at most, when asking by words. Fewer, more often, keeps the context small.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), handler.find)
}

// discovery is the tool's own state: what may be handed out, the server to hand it out on,
// and what that server offers from the start.
type discovery struct {
	catalogue *Catalogue
	server    *server.MCPServer
	already   map[string]bool
}

// has says whether this session can already call a tool — because the connection started with
// it, or because an earlier ask added it.
func (d *discovery) has(session server.ClientSession, name string) bool {
	if d.already[name] {
		return true
	}

	withTools, ok := session.(server.SessionWithTools)
	if !ok {
		return false
	}
	_, added := withTools.GetSessionTools()[name]

	return added
}

func (d *discovery) find(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session := server.ClientSessionFromContext(ctx)
	if session == nil {
		return toolError(fmt.Errorf("this connection has no session, so tools cannot be added to it; "+
			"connect to the ordinary path instead, which offers all %d at once", len(d.catalogue.tools))), nil
	}

	wanted, err := d.wanted(req, session)
	if err != nil {
		return toolError(err), nil
	}
	if len(wanted) == 0 {
		return resultJSON(map[string]any{
			"added": []string{},
			"note": "nothing new matched — either this session already has what you asked for, or " +
				"the words did not land. Tool descriptions are in English, so ask in English: " +
				"\"nested list\", \"cell colour\", \"speaker notes\". Or pass names with the exact " +
				"identifiers if you have them",
		})
	}

	adding := make([]server.ServerTool, 0, len(wanted))
	described := make([]map[string]any, 0, len(wanted))
	for _, name := range wanted {
		tool := d.catalogue.tools[name]
		adding = append(adding, *tool)
		described = append(described, map[string]any{
			"name": name, "does": tool.Tool.Description,
		})
	}

	if err := d.server.AddSessionTools(session.SessionID(), adding...); err != nil {
		return toolError(fmt.Errorf("this client cannot be given tools during a session (%w), which is "+
			"what discovery needs: connect without the discovery switch and the whole set arrives at "+
			"once", err)), nil
	}

	return resultJSON(map[string]any{
		"added":     described,
		"available": len(d.catalogue.tools),
		"note": "these are callable now. If your client does not list them yet, it has not re-read " +
			"the tool list — reconnect without the discovery switch and everything arrives at once",
	})
}

// wanted works out which tools to hand over.
func (d *discovery) wanted(req mcp.CallToolRequest, session server.ClientSession) ([]string, error) {
	if names := req.GetStringSlice("names", nil); len(names) > 0 {
		var unknown []string
		var found []string
		for _, name := range names {
			if _, ok := d.catalogue.tools[name]; !ok {
				unknown = append(unknown, name)
				continue
			}
			// A name asked for twice is not an error, it is a caller being careful.
			if !d.has(session, name) {
				found = append(found, name)
			}
		}
		if len(unknown) > 0 {
			return nil, fmt.Errorf("no tool called %s here; either the name is wrong or it removes "+
				"something, which never arrives by asking. Ask by words with about instead",
				strings.Join(unknown, ", "))
		}

		return found, nil
	}

	about := strings.TrimSpace(optionalString(req, "about"))
	if about == "" {
		return nil, fmt.Errorf("say what you are trying to do in about, or name the tools in names")
	}

	limit := req.GetInt("limit", 8)
	if limit < 1 {
		limit = 1
	}

	words := searchWords(about)
	if len(words) == 0 {
		// The descriptions are English, deliberately: they are read by a model and end up in
		// transcripts and issues. A question in another language matches nothing at all, and
		// saying so beats an empty answer that reads like "there is no such tool".
		return nil, fmt.Errorf("nothing in %q is a word this can search by. Tool names and "+
			"descriptions here are in English — ask in English, in the words of the job: "+
			"\"nested list\", \"picture as a slide background\", \"copy a tab into another workbook\"",
			about)
	}

	return d.search(words, limit, session), nil
}

// searchWords cuts a question into the words worth matching. Short ones are dropped: "a" and
// "in" appear in every description and would rank everything equally.
func searchWords(about string) []string {
	var words []string
	for _, word := range strings.FieldsFunc(strings.ToLower(about), func(r rune) bool {
		return !('a' <= r && r <= 'z') && !('0' <= r && r <= '9')
	}) {
		if len(word) >= 3 {
			words = append(words, word)
		}
	}

	return words
}

// search ranks the catalogue against a few words.
//
// The descriptions are what makes this work at all: they are long and say what a tool is for
// rather than what it is called, so "picture as a slide background" finds the tool whose
// description talks about stretching a picture over a slide, and no name would have matched.
func (d *discovery) search(words []string, limit int, session server.ClientSession) []string {
	type scored struct {
		name  string
		score int
	}
	var ranked []scored

	for name, tool := range d.catalogue.tools {
		// What the session can already call is not an answer to "what do I need". Half the
		// readings have "list" in the name, so without this a question about a nested list
		// comes back as seven listing tools the caller already had.
		if d.has(session, name) {
			continue
		}

		lowerName := strings.ToLower(name)
		lowerText := strings.ToLower(tool.Tool.Description)

		score := 0
		for _, word := range words {
			// A word in the name counts for more than a word in the description: a caller
			// who says "table" wants the table tools before the ones that mention tables in
			// passing.
			if strings.Contains(lowerName, word) {
				score += 4
			}
			if strings.Contains(lowerText, word) {
				score++
			}
		}
		if score > 0 {
			ranked = append(ranked, scored{name: name, score: score})
		}
	}

	// By score, then by name: the same question has to give the same answer twice running, or
	// a caller cannot tell a new match from a reshuffle.
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}

		return ranked[i].name < ranked[j].name
	})

	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	names := make([]string, 0, len(ranked))
	for _, one := range ranked {
		names = append(names, one.name)
	}

	return names
}

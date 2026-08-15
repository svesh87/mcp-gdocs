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

// Catalogue is what discovery may hand out: every tool the configuration allows.
type Catalogue struct {
	tools map[string]*server.ServerTool
}

// NewCatalogue takes the tools of a fully-registered server and offers all of them.
//
// Removal is in it, and the earlier rule that kept it out was premature. The ceiling is set
// by an operator at startup, in --tools; a catalogue that cuts removal out of that ceiling
// adds no safety, because the operator who typed slides-delete meant it. What it adds is
// detours: an agent that cannot ask for the one tool that takes a stray shape off a slide
// rebuilds the slide instead, and rebuilding is the operation that loses work. The finer
// permissions — a whole slide, a whole tab — are still checked inside the removing tool
// itself, on what a particular call named.
func NewCatalogue(full *server.MCPServer) *Catalogue {
	catalogue := &Catalogue{tools: map[string]*server.ServerTool{}}

	for name, tool := range full.ListTools() {
		if _, err := GroupOf(name); err != nil {
			continue
		}
		catalogue.tools[name] = tool
	}

	return catalogue
}

// offers says whether the catalogue holds anything at all from a group, which is how a wrong
// name is told apart from a group the server was started without.
func (d *discovery) offers(group Group) bool {
	for name := range d.catalogue.tools {
		if held, err := GroupOf(name); err == nil && held == group {
			return true
		}
	}

	return false
}

// removes says whether a name is one of the removing tools, for the answer to say so.
func removes(name string) bool {
	group, err := GroupOf(name)

	return err == nil && strings.Contains(string(group), "delete")
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
			"and this one; everything else — creating, styling, filling, exporting, removing — arrives "+
			"when you ask for it here, and stays for the rest of the session. Ask in the words of the "+
			"job: \"bullets\", \"picture as a slide background\", \"dropdown\", \"copy a tab into another "+
			"workbook\". English is what the names and descriptions are written in, and a short Russian "+
			"word of the trade is understood too. The answer carries each tool's arguments in full, so "+
			"the next call can be the real one — and gdocs_call_tool will make that call whether or not "+
			"your client has noticed the new name. Asking twice for the same thing is free. What was "+
			"never allowed at startup is not here: the answer says which group an operator would have "+
			"to add."),
		mcp.WithString("about", mcp.Description(
			"What you are trying to do, in a few words. Matched against every tool's name and its "+
				"description, which are in English; the common Russian words of the trade — таблица, "+
				"список, ссылка, картинка, диаграмма, вкладка, шрифт, цвет — are translated before "+
				"matching.")),
		mcp.WithArray("names", mcp.WithStringItems(), mcp.Description(
			"Exact tool names, when you already know them — from a skill, or from an earlier answer.")),
		mcp.WithNumber("limit", mcp.DefaultNumber(8), mcp.Description(
			"How many to add at most, when asking by words. Fewer, more often, keeps the context small.")),
		mcp.WithReadOnlyHintAnnotation(true),
	), handler.find)

	srv.AddTool(mcp.NewTool("gdocs_call_tool",
		mcp.WithDescription("Call any tool of this server by name, with its arguments as an object. "+
			"This is the way through when a tool has been added to the session but your client is still "+
			"showing the list it took at connection time: the name you cannot see, you can still call "+
			"here. Take the name and the arguments from what gdocs_find_tools answered. It reaches "+
			"exactly what the server was started with and nothing more, and the tool it calls does its "+
			"own checking, so a removal that needs an operator's permission is refused here too."),
		mcp.WithString("name", mcp.Required(), mcp.Description(
			"Tool to call, e.g. gdocs_sheets_update_chart.")),
		mcp.WithObject("arguments", mcp.Description(
			"The tool's own arguments, as an object. Omit it for a tool that takes none."),
			mcp.Properties(map[string]any{})),
	), handler.call)
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

		// The arguments come with the answer because the alternative is a second round
		// trip that a client may never make: it can only show a tool's arguments once it
		// has re-read the list. Calling a tool without knowing its arguments is worse than
		// slow — gdocs_sheets_update_chart writes a chart's whole specification back, and
		// a call made from a guess is what erases the data it draws.
		one := map[string]any{
			"name":      name,
			"does":      tool.Tool.Description,
			"arguments": tool.Tool.InputSchema,
		}
		if removes(name) {
			one["removes"] = true
		}
		described = append(described, one)
	}

	if err := d.server.AddSessionTools(session.SessionID(), adding...); err != nil {
		return toolError(fmt.Errorf("this client cannot be given tools during a session (%w), which is "+
			"what discovery needs: connect without the discovery switch and the whole set arrives at "+
			"once", err)), nil
	}

	return resultJSON(map[string]any{
		"added":     described,
		"available": len(d.catalogue.tools),
		// What used to stand here said "these are callable now", and that was a promise this
		// server cannot keep. A tool added to a session is announced to the client, and
		// whether the client acts on the announcement is the client's business: it has to be
		// listening on a stream it opened itself, and then re-read the list. Until it does,
		// the name is real and invisible — which reads, from the agent's side, exactly like
		// a tool that does not exist.
		"note": "added to this session. Your client may not show them until it re-reads its tool " +
			"list, and some clients never do — call them through gdocs_call_tool meanwhile, with " +
			"the name and the arguments above",
	})
}

// call runs a tool by name for a client that cannot see it yet.
func (d *discovery) call(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := requiredString(req, "name")
	if err != nil {
		return toolError(err), nil
	}

	tool, ok := d.catalogue.tools[name]
	if !ok {
		return toolError(d.unknown([]string{name})), nil
	}

	arguments := map[string]any{}
	if raw, given := req.GetArguments()["arguments"]; given && raw != nil {
		arguments, ok = raw.(map[string]any)
		if !ok {
			return toolError(fmt.Errorf("arguments has to be an object of the tool's own arguments, "+
				"not %T", raw)), nil
		}
	}

	inner := mcp.CallToolRequest{Header: req.Header}
	inner.Params.Name = name
	inner.Params.Arguments = arguments

	return tool.Handler(ctx, inner)
}

// unknown explains a name this server does not offer.
//
// A name can be missing for two different reasons, and the answer has to tell them apart. It
// may not exist — a typo, or a tool this server has never had. Or it may exist in the build
// and have been left out of --tools at startup, which is an operator's decision an agent
// cannot undo and should stop trying to work around. The name itself says which group it
// would belong to, so the second case can name what an operator would add — and removing a
// whole slide or tab needs the finer group on top of the family's own.
func (d *discovery) unknown(names []string) error {
	var missing []string
	for _, name := range names {
		group, err := GroupOf(name)
		// A group this catalogue already offers is proof the name itself is wrong: the
		// tools of that group are here, and this is not one of them. Saying "the operator
		// left it out" there would send an agent to argue with a configuration that is
		// already what it wants.
		if err != nil || d.offers(group) {
			continue
		}
		advice := string(group)
		if finer, ok := pageGroups[group]; ok {
			// In that family's own words: "a whole slide or tab" is two different things
			// and only one of them is ever the right one.
			advice += ", and " + string(finer) + " as well for " + pageWords[finer].whole
		}
		missing = append(missing, fmt.Sprintf("%s would be %s", name, advice))
	}

	if len(missing) == 0 {
		return fmt.Errorf("no tool called %s here — the name is wrong, or it belongs to another "+
			"family's window. Ask by words with about instead", strings.Join(names, ", "))
	}

	return fmt.Errorf("no tool called %s here. Either the name is wrong, or the server was started "+
		"without the group it belongs to (%s) — that is a decision made at startup with --tools, and "+
		"asking cannot change it. Ask by words with about to see what this server does offer",
		strings.Join(names, ", "), strings.Join(missing, "; "))
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
			return nil, d.unknown(unknown)
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
			"\"nested list\", \"picture as a slide background\", \"copy a tab into another workbook\". "+
			"The plain Russian nouns of the trade are understood too — таблица, список, ссылка, "+
			"картинка, диаграмма, вкладка, шрифт, цвет, фон, заголовок",
			about)
	}

	return d.search(words, limit, session), nil
}

// tradeWords translates the words this work is talked about in.
//
// Not a dictionary and not a translator: the sessions that drive this server are held in
// Russian, and an agent asked to fix a table on a slide reaches for the word it was asked in.
// The tool names and descriptions are English on purpose — they are read by a model and end
// up in transcripts — so a Russian question matched nothing at all, which reads as "there is
// no such tool" rather than "ask differently". These are the two dozen nouns that name the
// things a document is made of; a word that is not here still gets the refusal that says
// which language to ask in.
//
// Keys are stems, matched as prefixes, because the same noun arrives in any case Russian
// grammar puts it in — таблица, таблицу, таблице.
var tradeWords = map[string]string{
	"таблиц":    "table",
	"ячейк":     "cell",
	"строк":     "row",
	"столб":     "column",
	"колонк":    "column",
	"списк":     "list",
	"список":    "list",
	"маркер":    "bullet",
	"ссылк":     "link",
	"картинк":   "image",
	"изображен": "image",
	"рисун":     "image",
	"диаграмм":  "chart",
	"график":    "chart",
	"слайд":     "slide",
	"вкладк":    "tab",
	"лист":      "sheet",
	"книг":      "spreadsheet",
	"документ":  "document",
	"колод":     "presentation",
	"презентац": "presentation",
	"шрифт":     "font",
	"цвет":      "color",
	"фон":       "background",
	"заголов":   "title",
	"подпис":    "caption",
	"абзац":     "paragraph",
	"отступ":    "indent",
	"границ":    "border",
	"рамк":      "border",
	"заметк":    "notes",
	"шаблон":    "template",
	"макет":     "layout",
	"тем":       "theme",
	"колонтит":  "header footer",
	"формул":    "formula",
	"фильтр":    "filter",
	"выпадающ":  "dropdown",
	"папк":      "folder",
	"файл":      "file",
	"копи":      "copy",
	"удал":      "delete",
	"верси":     "revision",
	"коммент":   "comment",
	"доступ":    "share",
	"текст":     "text",
	"стиль":     "style",
	"стил":      "style",
	"размер":    "size",
	"вырав":     "alignment",
	"объедин":   "merge",
	"видео":     "video",
}

// searchWords cuts a question into the words worth matching. Short ones are dropped: "a" and
// "in" appear in every description and would rank everything equally.
func searchWords(about string) []string {
	var words []string
	for _, word := range strings.FieldsFunc(strings.ToLower(about), func(r rune) bool {
		return !('a' <= r && r <= 'z') && !('0' <= r && r <= '9') && !('а' <= r && r <= 'я') && r != 'ё'
	}) {
		if english, ok := translateWord(word); ok {
			words = append(words, strings.Fields(english)...)
			continue
		}
		// A Russian word that is not in the table matches nothing in an English description,
		// and passing it on would only dilute the ranking.
		if len(word) >= 3 && word[0] < 0x80 {
			words = append(words, word)
		}
	}

	return words
}

// translateWord turns one word of the trade into the English the descriptions are written in.
func translateWord(word string) (string, bool) {
	for stem, english := range tradeWords {
		if strings.HasPrefix(word, stem) {
			return english, true
		}
	}

	return "", false
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

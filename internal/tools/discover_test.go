package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// discoveryPair builds the two servers a discovery connection sits between: the full one,
// which is the catalogue, and the narrow one an agent actually talks to.
func discoveryPair(t *testing.T) (*server.MCPServer, *Catalogue) {
	t.Helper()

	full := serverWith(t, everyGroup(), true)
	narrow, catalogue := NarrowFrom(full, DiscoveryGroups(everyGroup()))

	return narrow, catalogue
}

// TestTheNarrowServerIsTheSameToolsAsTheFullOne is why the twin is built out of the original
// rather than registered again. Two registries of the same set can drift — a tool added to one
// and not the other is a difference nobody sees until an agent asks for what the catalogue
// promised and gets a handler that was never wired.
func TestTheNarrowServerIsTheSameToolsAsTheFullOne(t *testing.T) {
	full := serverWith(t, everyGroup(), true)
	narrow, _ := NarrowFrom(full, DiscoveryGroups(everyGroup()))

	for name, tool := range narrow.ListTools() {
		// The two tools discovery is made of exist only on the narrow server: one hands the
		// others out, the other calls them for a client whose list has not caught up.
		if name == "gdocs_find_tools" || name == "gdocs_call_tool" {
			continue
		}
		original, ok := full.ListTools()[name]
		if !ok {
			t.Errorf("%s is offered on the narrow server and does not exist on the full one", name)
			continue
		}
		if tool.Tool.Description != original.Tool.Description {
			t.Errorf("%s describes itself differently on the two servers", name)
		}
	}
}

// TestDiscoveryStartsSmall: the point of the whole mechanism is the size of the list an agent
// carries before it has called anything.
func TestDiscoveryStartsSmall(t *testing.T) {
	narrow, catalogue := discoveryPair(t)

	offered := len(narrow.ListTools())
	if offered >= len(catalogue.tools) {
		t.Errorf("a discovery connection offers %d of %d tools, which saves nothing",
			offered, len(catalogue.tools))
	}

	// The reference and this tool are always there: the first says what values the others'
	// arguments take, the second is how anything else arrives.
	for _, want := range []string{"gdocs_reference", "gdocs_find_tools", "gdocs_call_tool"} {
		if _, ok := narrow.ListTools()[want]; !ok {
			t.Errorf("a discovery connection has to offer %s", want)
		}
	}

	// And the readings, because every job here starts by reading something.
	if _, ok := narrow.ListTools()["gdocs_slides_list"]; !ok {
		t.Error("a discovery connection should start with the readings")
	}
	// But nothing that writes.
	if _, ok := narrow.ListTools()["gdocs_slides_set_text"]; ok {
		t.Error("a discovery connection should not start with the writing tools")
	}
}

// TestRemovalReachesAsFarAsTheOperatorAllowed: the ceiling is --tools and nothing else. A
// catalogue built from a server started with a removal group hands that group out; one built
// from a server started without it cannot, because those tools were never registered.
//
// Hiding removal from a server that was told to offer it was the earlier rule, and it bought
// nothing: the operator who typed slides-delete meant it, and the agent that could not ask
// for it rebuilt the slide instead.
func TestRemovalReachesAsFarAsTheOperatorAllowed(t *testing.T) {
	_, catalogue := discoveryPair(t)

	removing := 0
	for _, name := range catalogue.Names() {
		if removes(name) {
			removing++
		}
	}
	if removing == 0 {
		t.Error("a server started with the removal groups should offer them through discovery too")
	}
	if len(catalogue.Names()) == 0 {
		t.Error("the catalogue is empty, so discovery would have nothing to offer")
	}

	// And the other way round: what --tools left out is not in the catalogue at all.
	withoutRemoval := serverWith(t, defaultGroups(), true)
	_, narrowed := NarrowFrom(withoutRemoval, DiscoveryGroups(defaultGroups()))
	for _, name := range narrowed.Names() {
		if removes(name) {
			t.Errorf("%s removes things and the server was started without its group", name)
		}
	}
}

// TestDiscoveryNeedsASession: without one there is nowhere to add the tools to, and answering
// "added" would be a lie the caller only discovers when the call fails.
func TestDiscoveryNeedsASession(t *testing.T) {
	narrow, _ := discoveryPair(t)

	tool, ok := narrow.ListTools()["gdocs_find_tools"]
	if !ok {
		t.Fatal("discovery is not registered")
	}

	result, err := tool.Handler(context.Background(), request(map[string]any{"about": "bullets"}))
	message := requireError(t, result, err)
	if !strings.Contains(message, "ordinary path") {
		t.Errorf("the refusal should point at the path that offers everything, got %s", message)
	}
}

// TestDiscoveryAsksInEnglish: the descriptions are English on purpose, so a question made of
// words that are neither English nor in the table of trade words matches nothing at all. An
// empty answer would read as "there is no such tool", which is a different and wrong
// conclusion.
func TestDiscoveryAsksInEnglish(t *testing.T) {
	narrow, _ := discoveryPair(t)
	tool := narrow.ListTools()["gdocs_find_tools"]

	result, err := tool.Handler(narrow.WithContext(context.Background(), &plainSession{}),
		request(map[string]any{"about": "надо сделать красиво и побыстрее"}))
	message := requireError(t, result, err)
	if !strings.Contains(message, "English") {
		t.Errorf("the refusal should say which language to ask in, got %s", message)
	}
}

// TestDiscoveryUnderstandsTheWordsOfTheTrade: the sessions that drive this server are held in
// Russian, and the noun an agent was asked in is the noun it searches with. The tool names are
// English and stay English — the question is translated, not the server.
func TestDiscoveryUnderstandsTheWordsOfTheTrade(t *testing.T) {
	full := serverWith(t, everyGroup(), true)
	narrow := serverWith(t, DiscoveryGroups(everyGroup()), true)
	finder := &discovery{catalogue: NewCatalogue(full), server: narrow, already: map[string]bool{}}

	for _, probe := range []struct {
		about string
		want  string
	}{
		{"вложенный список", "list"},
		{"цвет заливки ячейки таблицы", "table"},
		{"заметки докладчика к слайду", "notes"},
		{"выпадающий список в книге", "dropdown"},
	} {
		found := finder.search(searchWords(probe.about), 5, nil)
		if len(found) == 0 {
			t.Errorf("%q found nothing", probe.about)
			continue
		}
		matched := false
		for _, name := range found {
			if strings.Contains(name, probe.want) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("%q should reach a tool with %q in its name, got %v", probe.about, probe.want, found)
		}
	}
}

// TestDiscoveryRanksNamesOverDescriptions: a caller who says "table" wants the table tools,
// not the twenty whose descriptions mention a table in passing.
func TestDiscoveryRanksNamesOverDescriptions(t *testing.T) {
	full := serverWith(t, everyGroup(), true)
	narrow := serverWith(t, DiscoveryGroups(everyGroup()), true)
	finder := &discovery{catalogue: NewCatalogue(full), server: narrow, already: map[string]bool{}}

	found := finder.search(searchWords("speaker notes"), 3, nil)
	if len(found) == 0 || !strings.Contains(found[0], "speaker_notes") {
		t.Errorf("the tool named for the job should come first, got %v", found)
	}
}

// TestDiscoverySkipsWhatIsAlreadyThere is the fix a live session demanded. Half the readings
// have "list" in the name, so a question about a nested list came back as seven listing tools
// the connection already had — an answer that spends its whole budget saying nothing.
func TestDiscoverySkipsWhatIsAlreadyThere(t *testing.T) {
	full := serverWith(t, everyGroup(), true)
	narrow := serverWith(t, DiscoveryGroups(everyGroup()), true)

	already := map[string]bool{}
	for name := range narrow.ListTools() {
		already[name] = true
	}
	finder := &discovery{catalogue: NewCatalogue(full), server: narrow, already: already}

	for _, name := range finder.search(searchWords("nested bullet list"), 8, nil) {
		if already[name] {
			t.Errorf("%s is already offered on this connection and should not be an answer", name)
		}
	}
}

// TestDiscoveryAddsToTheAskingSessionOnly is the mechanism end to end, and the last clause is
// the part that makes it safe to put behind a shared path: two clients on the same address get
// their own lists, and one asking for a tool does not hand it to the other.
func TestDiscoveryAddsToTheAskingSessionOnly(t *testing.T) {
	narrow, _ := discoveryPair(t)

	asking := &toolHoldingSession{id: "asking"}
	quiet := &toolHoldingSession{id: "quiet"}
	for _, session := range []*toolHoldingSession{asking, quiet} {
		if err := narrow.RegisterSession(context.Background(), session); err != nil {
			t.Fatalf("registering a session: %v", err)
		}
	}

	tool := narrow.ListTools()["gdocs_find_tools"]
	result, err := tool.Handler(narrow.WithContext(context.Background(), asking),
		request(map[string]any{"about": "speaker notes", "limit": float64(2)}))
	answer := requireOK(t, result, err)

	if !strings.Contains(answer, "gdocs_slides_set_speaker_notes") {
		t.Errorf("the answer should name what it added, got %s", answer)
	}
	if len(asking.tools) == 0 {
		t.Error("the tools should have been added to the session that asked")
	}
	if len(quiet.tools) != 0 {
		t.Errorf("the other session should be untouched, it has %d tools", len(quiet.tools))
	}

	// Asking again for the same thing is free, and answers with nothing rather than adding
	// it twice — a caller being careful should not be punished for it.
	repeated, err := tool.Handler(narrow.WithContext(context.Background(), asking),
		request(map[string]any{"names": []any{"gdocs_slides_set_speaker_notes"}}))
	again := requireOK(t, repeated, err)
	if !strings.Contains(again, `"added": []`) {
		t.Errorf("a tool already added should not be added again, got %s", again)
	}
}

// TestDiscoveryNamesTheGroupAnOperatorWouldAdd: an agent that asks for a tool the server was
// started without gets a refusal it can act on — not by retrying, but by saying which line
// the operator has to change. The old message stopped at "never arrives by asking", and what
// the agent did with that was look for a way round.
func TestDiscoveryNamesTheGroupAnOperatorWouldAdd(t *testing.T) {
	full := serverWith(t, defaultGroups(), true)
	narrow, _ := NarrowFrom(full, DiscoveryGroups(defaultGroups()))

	session := &toolHoldingSession{id: "asking"}
	if err := narrow.RegisterSession(context.Background(), session); err != nil {
		t.Fatalf("registering a session: %v", err)
	}

	tool := narrow.ListTools()["gdocs_find_tools"]
	refused, err := tool.Handler(narrow.WithContext(context.Background(), session),
		request(map[string]any{"names": []any{"gdocs_slides_delete"}}))
	message := requireError(t, refused, err)

	for _, want := range []string{"slides-delete", "slides-delete-page", "--tools"} {
		if !strings.Contains(message, want) {
			t.Errorf("the refusal should mention %s, got %s", want, message)
		}
	}
}

// TestDiscoveryAnswersWithTheArguments: a name and a sentence are not enough to call anything
// with. gdocs_sheets_update_chart writes a chart's whole specification back, and a call built
// from a guess at its arguments is what erases the data the chart draws.
func TestDiscoveryAnswersWithTheArguments(t *testing.T) {
	narrow, _ := discoveryPair(t)

	session := &toolHoldingSession{id: "asking"}
	if err := narrow.RegisterSession(context.Background(), session); err != nil {
		t.Fatalf("registering a session: %v", err)
	}

	tool := narrow.ListTools()["gdocs_find_tools"]
	result, err := tool.Handler(narrow.WithContext(context.Background(), session),
		request(map[string]any{"names": []any{"gdocs_slides_set_speaker_notes"}}))
	answer := requireOK(t, result, err)

	for _, want := range []string{`"arguments"`, "presentation_id", "gdocs_call_tool"} {
		if !strings.Contains(answer, want) {
			t.Errorf("the answer should carry %s, got %s", want, answer)
		}
	}
	if strings.Contains(answer, "callable now") {
		t.Error("the answer should not promise a callability this server cannot deliver")
	}
}

// TestCallToolIsTheWayThrough is the whole reason that tool exists: a client that has not
// re-read its list cannot call a tool by name, and the notification that would have told it
// to re-read goes down a stream the client may never open. This way needs nothing of the
// client at all.
func TestCallToolIsTheWayThrough(t *testing.T) {
	narrow, _ := discoveryPair(t)

	tool, ok := narrow.ListTools()["gdocs_call_tool"]
	if !ok {
		t.Fatal("a discovery connection has to offer gdocs_call_tool")
	}

	// A tool that never reaches Google: it refuses on its own arguments, which is enough to
	// prove the call got through to the right handler with the arguments it was given.
	result, err := tool.Handler(context.Background(), request(map[string]any{
		"name":      "gdocs_slides_set_speaker_notes",
		"arguments": map[string]any{"presentation_id": ""},
	}))
	message := requireError(t, result, err)
	if !strings.Contains(message, "presentation_id") {
		t.Errorf("the inner tool should have seen the arguments, got %s", message)
	}

	// A name outside the catalogue is refused the same way as in a search, so the two
	// answers cannot contradict each other. This one's group is offered, so the name is
	// simply wrong — and the refusal must not blame a configuration that is already right.
	refused, err := tool.Handler(context.Background(), request(map[string]any{
		"name": "gdocs_slides_no_such_thing",
	}))
	message = requireError(t, refused, err)
	if !strings.Contains(message, "the name is wrong") {
		t.Errorf("an unknown name should be refused plainly, got %s", message)
	}
	if strings.Contains(message, "--tools") {
		t.Errorf("a typo should not be reported as a missing group, got %s", message)
	}

	// Arguments of the wrong shape are a refusal rather than a panic.
	wrong, err := tool.Handler(context.Background(), request(map[string]any{
		"name": "gdocs_slides_set_speaker_notes", "arguments": "presentation_id=1",
	}))
	if message := requireError(t, wrong, err); !strings.Contains(message, "object") {
		t.Errorf("arguments that are not an object should say so, got %s", message)
	}
}

// toolHoldingSession is a session that can be given tools, as the HTTP transport's is.
type toolHoldingSession struct {
	id    string
	tools map[string]server.ServerTool
}

func (s *toolHoldingSession) Initialize()       {}
func (s *toolHoldingSession) Initialized() bool { return true }
func (s *toolHoldingSession) NotificationChannel() chan<- mcp.JSONRPCNotification {
	return make(chan mcp.JSONRPCNotification, 8)
}
func (s *toolHoldingSession) SessionID() string { return s.id }
func (s *toolHoldingSession) GetSessionTools() map[string]server.ServerTool {
	return s.tools
}
func (s *toolHoldingSession) SetSessionTools(tools map[string]server.ServerTool) {
	s.tools = tools
}

// plainSession is a client session with no per-session tool storage, which is what a
// connection that cannot be given tools during a session looks like.
type plainSession struct{}

func (s *plainSession) Initialize()       {}
func (s *plainSession) Initialized() bool { return true }
func (s *plainSession) NotificationChannel() chan<- mcp.JSONRPCNotification {
	return make(chan mcp.JSONRPCNotification, 1)
}
func (s *plainSession) SessionID() string { return "probe" }

package auth

import (
	"html/template"
	"net/http"
	"strings"
	"time"
)

// The sign-in page exists because consent can only be given by the person whose account
// the server will act as, in their own browser. When the server runs in a container there
// is no browser inside it, so the page is served on the published loopback port and the
// person's own browser does the rest.
//
// Nothing about the token is shown, not even truncated: the page is rendered in a browser
// and browsers keep history. What it shows is whether a sign-in is needed, which scopes
// are being asked for, and where the token file lives.
//
// Its text is in Russian while the rest of this repository is in English, and that split
// is deliberate: this page is read by the people who sign in, while comments and error
// messages end up in agent transcripts and issues.

// pageView is what the template renders.
type pageView struct {
	Key      string
	Status   Status
	Scopes   []string
	Redirect string
	Error    string
	Done     bool
}

// SessionLine describes the stored token in words.
func (v pageView) SessionLine() string {
	switch {
	case !v.Status.SignedIn:
		return "входа ещё не было"
	case v.Status.AccessValid && v.Status.AccessLeft > 0:
		return "доступ действует, осталось " + humanDuration(v.Status.AccessLeft)
	case v.Status.AccessValid:
		return "доступ действует"
	default:
		return "доступ истёк, но продлевается сам"
	}
}

// render writes the page.
func (l *WebLogin) render(w http.ResponseWriter, view pageView) {
	if view.Error != "" && !view.Done {
		w.WriteHeader(http.StatusBadRequest)
	}

	// The response is on its way by the time a template error could happen, so there is
	// nothing useful left to say to the browser.
	_ = loginTemplate.Execute(w, view)
}

// humanDuration is a duration in words, rounded the way a person reads it.
func humanDuration(left time.Duration) string {
	switch {
	case left >= time.Hour:
		return left.Round(time.Minute).String()
	case left >= time.Minute:
		return left.Round(time.Second).String()
	default:
		return "меньше минуты"
	}
}

// The page is one file with no external anything: it is served on loopback by a server
// built FROM scratch, and a page that fetches a font would simply fail.
var loginTemplate = template.Must(template.New("login").Parse(strings.TrimSpace(`
<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>Вход в Google · mcp-gdocs</title>
<style>
:root { color-scheme: light dark; }
body {
  font: 16px/1.5 system-ui, sans-serif;
  max-width: 34rem; margin: 3rem auto; padding: 0 1rem;
}
h1 { font-size: 1.3rem; margin-bottom: .3rem; }
p.lead { color: GrayText; margin-top: 0; }
form { margin: 1.5rem 0; }
button {
  font: inherit; padding: .6rem 1rem; border: 0; border-radius: .4rem;
  background: Highlight; color: HighlightText; cursor: pointer;
}
.box { border: 1px solid GrayText; border-radius: .5rem; padding: .8rem 1rem; margin: 1rem 0; }
.bad { border-color: #c0392b; }
.good { border-color: #2e7d32; }
code { font-size: .9em; overflow-wrap: anywhere; }
ul { margin: .3rem 0; padding-left: 1.2rem; }
</style>
</head>
<body>
<h1>Вход в Google</h1>
<p class="lead">
  Сервер работает от имени того, кто здесь вошёл. Пароль он не видит и не хранит: браузер
  уходит на страницу согласия Google, а обратно возвращается только разрешение, из которого
  сервер делает токен.
</p>

<div class="box">
  <div>{{ .SessionLine }}</div>
  {{ if .Status.Path }}<div><code>{{ .Status.Path }}</code></div>{{ end }}
  {{ if .Status.Scopes }}
  <div>права токена:</div>
  <ul>{{ range .Status.Scopes }}<li><code>{{ . }}</code></li>{{ end }}</ul>
  {{ end }}
</div>

{{ if .Error }}<div class="box bad">{{ .Error }}</div>{{ end }}
{{ if .Done }}<div class="box good">Вход выполнен, токен сохранён. Вкладку можно закрыть.</div>{{ end }}

{{ if .Redirect }}
<form method="post" autocomplete="off">
  <input type="hidden" name="key" value="{{ .Key }}">
  <button type="submit">{{ if .Status.SignedIn }}Войти заново{{ else }}Войти через Google{{ end }}</button>
</form>

<p class="lead">
  Будут запрошены права:
</p>
<ul>{{ range .Scopes }}<li><code>{{ . }}</code></li>{{ end }}</ul>

<p class="lead">
  Google вернёт браузер на <code>{{ .Redirect }}</code> — это тот же порт, на котором открыта
  эта страница, поэтому код авторизации не покидает машину.
</p>
{{ end }}
</body>
</html>
`)))

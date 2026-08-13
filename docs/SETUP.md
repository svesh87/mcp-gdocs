*English · [Русский](SETUP.ru.md)*

# Google Workspace setup

Everything here is done once, in [Google Cloud Console](https://console.cloud.google.com/),
by a person with a Google Workspace account in the organisation the server will act
inside. Fifteen minutes, and the last step needs a browser.

Google renamed the OAuth consent screen to **Google Auth Platform** in 2025. If a guide
you find elsewhere talks about "APIs & Services → OAuth consent screen", it predates
that; the names below are the current ones.

## 1. A project of its own

**Menu → IAM & Admin → Create a Project**

| Field | What to put in it |
|---|---|
| **Project name** | anything readable, e.g. `mcp-gdocs`. Can be changed later |
| **Project ID** | generated from the name. **Cannot be changed after creation** — click *Edit* now if it matters |
| **Location** | your **organisation**, not *No organization* |

The location is the one field worth stopping at. An **Internal** audience — the whole
security model of the next steps — is only available to a project owned by an
organisation. A project created outside one cannot be made Internal afterwards without
recreating it.

A new project rather than an existing one: the OAuth client and its consent screen belong
to the project, and an old project carries settings nobody remembers making.

Billing is not needed. These four APIs have no paid tier to enable.

## 2. Turn on four APIs

**Menu → APIs & Services → Library**, then for each of the four: search it, open it,
click **Enable**.

| API | Service identifier |
|---|---|
| Google Drive API | `drive.googleapis.com` |
| Google Sheets API | `sheets.googleapis.com` |
| Google Slides API | `slides.googleapis.com` |
| Google Docs API | `docs.googleapis.com` |

The search returns near neighbours; go by the exact name and the identifier. Searching
for Drive also offers **Drive Labels API** (label taxonomies), **Google Drive Activity
API** (who opened and edited what) and **Google Drive MCP** (Google's own MCP endpoint
for Drive, unrelated to this server). None of the three is needed: this server calls the
Drive API itself.

Or in one line, if you have the CLI:

```bash
gcloud services enable drive.googleapis.com sheets.googleapis.com \
                       slides.googleapis.com docs.googleapis.com --project=<PROJECT_ID>
```

An API left off answers every call with a 403 that names the API and links to the page
that enables it — at least it is an error that says what to do.

## 3. Branding

**Menu → APIs & Services → OAuth consent screen**, which opens the Google Auth Platform.
It is not a top-level entry in the console menu — expand *APIs & Services* to find it. In
a project where it has never been opened, the first visit is a short wizard covering this
step and the next one in one form.

Going straight there beats hunting through the menu:

```
https://console.cloud.google.com/auth/branding?project=<PROJECT_ID>
https://console.cloud.google.com/auth/audience?project=<PROJECT_ID>
https://console.cloud.google.com/auth/clients?project=<PROJECT_ID>
```

**Google Auth Platform → Branding**

| Field | What to put in it |
|---|---|
| **App name** | what the person sees on the consent screen, e.g. `mcp-gdocs` |
| **User support email** | your address, or a group. Shown on the consent screen |
| **App logo** | skip |
| **App domain**, **Authorised domains** | skip — these matter for External apps that get verified |
| **Contact information → Email address** | where Google writes about the project. Your address |

Then tick **I agree to the Google API Services: User Data Policy** and finish.

## 4. Audience: Internal

**Google Auth Platform → Audience → User type: Internal**
([direct link](https://console.cloud.google.com/auth/audience))

This is the decision that makes the rest simple. With Internal:

- only accounts in your organisation can complete the consent — a leaked client
  identifier is not a way in for anyone outside it;
- no verification review, even though `drive` is a restricted scope. For an External app
  the same scope means Google's review and a security assessment;
- no "unverified app" warning screen, and no 100-user cap.

**External** is the fallback if the account is a personal `@gmail.com` one rather than a
Workspace account. It works, with two differences: every account that signs in has to be
added under **Audience → Test users**, and the consent screen carries the unverified-app
warning. For a server one person runs for themselves, that is tolerable.

## 5. An OAuth client of type Desktop

**Google Auth Platform → Clients → Create client**
([direct link](https://console.cloud.google.com/auth/clients))

| Field | Value |
|---|---|
| **Application type** | **Desktop app** |
| **Name** | anything, e.g. `mcp-gdocs desktop`. Console-only, nobody else sees it |

Then **Create**, and **Download JSON** on the client that appeared.

Desktop is not a cosmetic choice. Google lets a client of this type redirect to any
loopback address **without registering it in the console**, and that is what makes both
sign-in routes work with nothing else to configure: the `login` command's temporary port,
and a container's published port on `127.0.0.1`. A **Web application** client would
require every redirect address to be listed in advance, and the sign-in below would fail
with `redirect_uri_mismatch`.

The download is called `client_secret_<long id>.apps.googleusercontent.com.json`. Rename
it to `gcp-oauth.keys.json` and put it where the server will read it. Inside:

```json
{"installed": {
  "client_id": "…apps.googleusercontent.com",
  "client_secret": "…",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token"
}}
```

The server reads `installed` (and accepts `web`, so a wrong client type fails with a
clear message at sign-in rather than a parse error). `auth_uri` and `token_uri` are
optional — Google's own are the default.

**Or skip the file entirely.** The only two things in it that matter are the identifier
and the secret, so they can be handed over as environment variables instead:

```bash
GOOGLE_OAUTH_CLIENT_ID=…apps.googleusercontent.com
GOOGLE_OAUTH_CLIENT_SECRET=…
```

That is the shape a password store hands over, and it leaves nothing about the
application on disk. Take the two values out of the downloaded file, put them wherever
secrets live, and delete the file. If both a file and the variables are present, the
variables win.

## 6. Scopes

**Google Auth Platform → Data Access**
([direct link](https://console.cloud.google.com/auth/scopes)) — and for an Internal app
there is nothing to do on it. The scopes are requested by the server itself at sign-in,
and for Internal they are neither shown on the consent screen nor reviewed. The page
matters for External apps, where the declared list has to match what the app asks for:
**Add or remove scopes** picks from the scopes of the APIs enabled in step 2 — only those
are listed — and *Manually add scopes* takes anything missing.

The four the server asks for by default:

```
https://www.googleapis.com/auth/drive
https://www.googleapis.com/auth/spreadsheets
https://www.googleapis.com/auth/presentations
https://www.googleapis.com/auth/documents
```

The full `drive` rather than the narrower `drive.file` is deliberate: `drive.file` only
ever sees files this application itself created, so it cannot open the template a deck is
copied from. `--scopes` narrows the list without a rebuild, by alias or full URL:

```bash
mcp-gdocs --scopes presentations,drive.readonly …
```

Changing the scopes means signing in again — a token carries the scopes it was granted
with.

## 7. First sign-in

On a machine with a browser:

```bash
mcp-gdocs login --credentials ./gcp-oauth.keys.json --token-dir ./tokens
```

It prints an address, waits on a loopback port, and writes `tokens/token.json` when the
browser comes back. Sign in as the account the server should act as.

With the server in a container there is no browser inside it, so the sign-in happens
through the server's own page. Start it with `--transport=streamable-http`, publish the
port on loopback, and open:

```
http://127.0.0.1:8819/login?key=<MCP_AUTH_TOKEN>
```

The page says whether anybody is signed in, which scopes will be asked for and where the
token file is, and has one button. Pressing it sends the browser to Google's consent
screen; Google sends it back to `http://127.0.0.1:8819/oauth2callback` — your own
published port — so the authorisation code never leaves the machine, and the page reports
that the token was saved.

Details that matter: the key is in the address because a browser sends no `Authorization`
header of its own, which is why the page is served `Cache-Control: no-store` and
`Referrer-Policy: no-referrer` — the key must not reach Google in a referrer or sit in a
cache. The trip to Google is a form submission rather than a link, so a `/login` address
forwarded to somebody else shows them a page instead of walking them into a consent
screen. The exchange is PKCE-protected (`code_challenge_method=S256`), and the sign-in
asks for offline access with a forced consent screen: without both, Google returns no
refresh token on a repeat sign-in and the server stops working within the hour.

**If the server runs on another machine**, do not expose that port publicly and open it by
hostname: Google only redirects a Desktop client to a loopback address, so the sign-in
would fail with `redirect_uri_mismatch`, and the page refuses it before that. Tunnel the
port instead, which makes the browser's address loopback again:

```bash
ssh -L 8819:127.0.0.1:8819 <host>
# then open http://127.0.0.1:8819/login?key=… in the local browser
```

## 8. Where the two files live

They are separate on purpose, and only one of them is a person's own:

| Secret | What it is | Where it lives |
|---|---|---|
| the OAuth client | the application: which app asks for consent | a file named by `--credentials`, or `GOOGLE_OAUTH_CLIENT_ID` and `GOOGLE_OAUTH_CLIENT_SECRET` in the environment. Shared out of band with whoever may run this server |
| `token.json` | one person's consent, turned into a refresh token | written by that person's own sign-in, 0600, in `--token-dir` |

The split follows what writes what. The client is read once at startup and never changes,
so it can come from a password store as two variables and never touch disk. The token is
rewritten by the server every time the access token is refreshed, so it is a file the
process owns — a secret store is not somewhere a program writes to on its own.

Neither is ever committed or built into an image: `.gitignore` and `.dockerignore` carry
both, and the build was checked to confirm neither reaches the build context.

## 9. Why there is no service account

A service account with domain-wide delegation would remove the sign-in entirely, and it
is exactly what this server does not do.

Such a key is the whole of the access, and delegation lets it act as any employee in the
Workspace without that person knowing or being able to see it in their own security
settings. An image or a repository that leaked one would be a compromise of every
document in the company, and revoking it means rotating a key half the stack depends on.

A personal token reaches exactly what one person can already reach, appears in that
person's own account settings, and is revoked there in one click.

## 10. If something leaks

**The token** — revoke it at
[myaccount.google.com/permissions](https://myaccount.google.com/permissions): find the
app, remove access. Every refresh token issued to it dies immediately. Delete
`token.json` and sign in again.

**The client secret** — in the console, **Clients → your client → Reset secret**, then
hand the new `gcp-oauth.keys.json` to whoever runs the server. Existing tokens keep
working until they refresh, so revoke them as above if the leak is serious. On an
Internal audience a leaked client identifier is not by itself a way in: consent still
requires an account in the organisation.

## 11. Errors worth recognising

| What it says | What it means |
|---|---|
| `no Google token yet` | nobody has signed in: run `login`, or open `/login?key=…` |
| `invalid_grant` when refreshing | consent was revoked, or the token was replaced. Sign in again |
| `redirect_uri_mismatch` | the client is not of type Desktop, or the sign-in was opened on something other than a loopback address |
| `403` naming an API | that API is not enabled in the project (step 2) |
| `403 PERMISSION_DENIED` on a file | the signed-in person cannot reach it — usually a shared drive they were never added to |
| `access_denied` on the consent screen | Internal audience and an account outside the organisation |
| `Google returned no refresh token` | a repeat consent without a forced screen. Revoke the app's access and sign in again |

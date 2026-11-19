package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/svesh87/mcp-gdocs/internal/google"
)

// slideImageTypes are the pictures createImage accepts, by extension. Slides takes PNG, JPEG
// and GIF and nothing else; an SVG or a WebP is refused by Google with a message about
// retrieving the image, which reads as a network fault and is not one.
var slideImageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
}

// slideImageLimit is the largest picture Slides will fetch: 50 MB.
const slideImageLimit = 50 << 20

// imageFetchAttempts and imageFetchDelay bound the waiting for a link that was opened a
// moment ago. Drive answers the grant before every one of its front ends has heard of it,
// and a fetch that arrives first is told the file is private. The delay is a variable so a
// test does not sit through it.
var (
	imageFetchAttempts = 3
	imageFetchDelay    = 2 * time.Second
)

// publishedPicture is what putting a local picture in front of Slides left behind.
type publishedPicture struct {
	// FileID is the copy on Drive. It stays there, private again, after the call.
	FileID string
	// Opened says a link was ever opened. An upload whose grant was refused was never
	// readable by anyone else, which is a different answer from "opened and closed".
	Opened bool
	// Closed says the link opened for the fetch was taken back. False is an alarm, not a
	// detail: the file is still readable by anyone holding the address.
	Closed bool
	// PermissionID names the grant, for the caller who has to take it back by hand.
	PermissionID string
	// CloseError is why it could not be taken back.
	CloseError string
}

// mayPublishPictures says whether this server may open a file by link at all.
func (r *registry) mayPublishPictures() bool {
	return r.opts.MayShare || r.enabled(DriveShare)
}

// withPublishedPicture puts a file from the files directory where Slides can fetch it, runs
// the insertion against that address, and closes the address again whatever happened.
//
// createImage takes an address and fetches it from Google's side, as nobody in particular:
// a file on a laptop, or a private file on Drive, is not something it can reach. The only
// route for a local picture is the one a person would take by hand — upload it, open it to
// anyone with the link, insert, close the link — and done by hand the last step is the one
// that gets forgotten. Here it is not a step a caller can forget: the grant is taken back on
// every path out of this function, and an answer that could not take it back says so first.
//
// Opening a file by link is sharing, however briefly. It therefore happens only on a server
// whose operator named drive-share, and the refusal names that group.
func (r *registry) withPublishedPicture(ctx context.Context, client *google.Client, name, folderID string,
	insert func(address string) error) (*publishedPicture, error) {
	path, err := r.filePath(name)
	if err != nil {
		return nil, err
	}

	if !r.mayPublishPictures() {
		return nil, fmt.Errorf("a picture from a file reaches a slide by being uploaded to Drive and " +
			"opened to anyone with the link for the moment Slides fetches it — Slides takes an address " +
			"and nothing else. That is sharing, and this server was started without " + string(DriveShare) +
			": add it to --tools, or pass url with an address Google can already fetch")
	}

	extension := strings.ToLower(filepath.Ext(path))
	mimeType, ok := slideImageTypes[extension]
	if !ok {
		return nil, fmt.Errorf("%s is not a picture Slides accepts: it takes PNG, JPEG and GIF, so "+
			"convert the file first", filepath.Base(path))
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("%s is empty", path)
	}
	if len(content) > slideImageLimit {
		return nil, fmt.Errorf("%s is %d bytes, and Slides fetches pictures of up to %d: make it smaller",
			path, len(content), slideImageLimit)
	}

	uploaded, err := client.UploadFile(ctx, filepath.Base(path), mimeType, "", folderID, content)
	if err != nil {
		return nil, fmt.Errorf("uploading %s to Drive: %w", filepath.Base(path), err)
	}

	published := &publishedPicture{FileID: uploaded.ID}

	grant, err := client.Share(ctx, uploaded.ID, google.SharePermission{Type: "anyone", Role: "reader"}, false)
	if err != nil {
		// Nothing was opened, so there is nothing to close; the copy is private.
		published.Closed = true

		return published, fmt.Errorf("the picture was uploaded as %s but could not be opened by link, "+
			"so Slides cannot fetch it (%w). An organisation that forbids sharing outside itself "+
			"answers this way; such a picture has to come from an address Google can reach, passed "+
			"as url. The uploaded copy is private and still on Drive", uploaded.ID, err)
	}
	published.PermissionID = grant.ID
	published.Opened = true

	address := "https://drive.google.com/uc?export=download&id=" + uploaded.ID

	var insertErr error
	for attempt := 0; attempt < imageFetchAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(imageFetchDelay):
			case <-ctx.Done():
				insertErr = ctx.Err()
			}
			if ctx.Err() != nil {
				break
			}
		}

		if insertErr = insert(address); insertErr == nil {
			break
		}
	}

	// Closed on a context of its own: a caller that gave up waiting is exactly the caller
	// whose link must not be left open behind it.
	closing, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	if err := client.Unshare(closing, uploaded.ID, grant.ID); err != nil {
		published.CloseError = err.Error()
	} else {
		published.Closed = true
	}

	return published, insertErr
}

// describe adds what the upload left behind to an answer.
func (p *publishedPicture) describe(payload map[string]any) {
	payload["uploaded_file_id"] = p.FileID

	if !p.Opened {
		payload["link_access"] = "never opened"
		payload["upload_note"] = "the uploaded copy was never opened to anyone and is still on " +
			"Drive, private, until somebody bins it with gdocs_drive_delete_to_trash"

		return
	}

	if p.Closed {
		payload["link_access"] = "closed"
		payload["upload_note"] = "the picture was uploaded to Drive, opened to anyone with the link " +
			"while Slides fetched it, and closed again. Slides keeps its own copy, so the upload is " +
			"no longer needed: it stays on Drive, private, until somebody bins it with " +
			"gdocs_drive_delete_to_trash"

		return
	}

	payload["link_access"] = "STILL OPEN"
	payload["upload_note"] = "the uploaded picture is still readable by anyone with its link: taking " +
		"the access back failed (" + p.CloseError + "). Close it now with gdocs_drive_unshare " +
		"file_id=" + p.FileID + " permission_id=" + p.PermissionID
}

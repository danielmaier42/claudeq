package main

import (
	"flag"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
)

// cmdNotify queues a notification for the daemon to send over the configured
// channels (macOS, Pushover). It is meant to be run by Claude from inside a
// task (see executor.notifySystemPrompt) but also works standalone. The daemon
// delivers it on its next tick, attributed to the calling task.
func cmdNotify(st *store.Store, args []string) error {
	fs := flag.NewFlagSet("notify", flag.ContinueOnError)
	title := fs.String("title", "", "notification title (required)")
	message := fs.String("message", "", "notification text (required)")
	link := fs.String("url", "", "optional http(s) link the notification opens")
	if err := fs.Parse(args); err != nil {
		return err
	}
	src := callingRun()
	now := time.Now()
	n, err := buildNotification(*title, *message, *link, src, now)
	if err != nil {
		return err
	}
	if err := st.QueueNotification(n); err != nil {
		return err
	}
	fmt.Printf("queued notification %q\n", n.Title)
	return nil
}

// buildNotification validates the caller's input and assembles the outbox
// record. Both title and message are required; the link, when given, must be
// an absolute http or https URL — that is what the channels can open.
func buildNotification(title, message, link string, src runSource, now time.Time) (store.Notification, error) {
	title = strings.TrimSpace(title)
	message = strings.TrimSpace(message)
	link = strings.TrimSpace(link)
	if title == "" {
		return store.Notification{}, fmt.Errorf("--title is required")
	}
	if message == "" {
		return store.Notification{}, fmt.Errorf("--message is required")
	}
	if link != "" {
		u, err := url.Parse(link)
		if err != nil {
			return store.Notification{}, fmt.Errorf("invalid --url: %w", err)
		}
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return store.Notification{}, fmt.Errorf("invalid --url %q: want an absolute http or https URL", link)
		}
	}
	return store.Notification{
		ID:       newNotificationID(now),
		Title:    title,
		Message:  message,
		URL:      link,
		TaskID:   src.taskID,
		TaskName: src.taskName,
		RunID:    src.runID,
		QueuedAt: now,
	}, nil
}

// newNotificationID builds a unique-ish id; the random suffix disambiguates
// several notifications queued within the same second.
func newNotificationID(now time.Time) string {
	return "n-" + now.UTC().Format("20060102T150405") + "-" + shortHex(3)
}

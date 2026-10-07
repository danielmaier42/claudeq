// The `claudeq artifacts` command group: the published artifacts, as the
// Artifacts view lists them, for scripts that pick up what a job delivered.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/store"
)

const artifactsUsage = `claudeq artifacts - files your tasks published

Usage:
  claudeq artifacts list [--json] [--origin ID] [--since TS]

--origin keeps the artifacts one parent job produced, itself or through the
jobs it created, as the parent filter in the Artifacts view does.
--since keeps the artifacts published after TS: an RFC3339 time, or a
duration such as 24h for "within the last 24 hours". Oldest first, so the
last entry's published_at is the TS for the next poll.`

func cmdArtifacts(st *store.Store, args []string) error {
	if len(args) == 0 {
		fmt.Println(artifactsUsage)
		return nil
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return cmdArtifactsList(st, rest, time.Now())
	default:
		fmt.Println(artifactsUsage)
		return fmt.Errorf("unknown artifacts command %q", sub)
	}
}

// listedArtifact is one artifact as `claudeq artifacts list --json` prints
// it: the stored record with its group and parent resolved as the Artifacts
// view shows them, the absolute path of the stored copy in place of the
// store-relative one, and the unread flag.
type listedArtifact struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	FileName    string    `json:"file_name"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	TaskID      string    `json:"task_id,omitempty"`
	TaskName    string    `json:"task_name,omitempty"`
	RunID       string    `json:"run_id,omitempty"`
	Group       string    `json:"group,omitempty"`
	OriginID    string    `json:"origin_id,omitempty"`
	OriginName  string    `json:"origin_name,omitempty"`
	PublishedAt time.Time `json:"published_at"`
	Unread      bool      `json:"unread"`
}

func cmdArtifactsList(st *store.Store, args []string, now time.Time) error {
	fs := flag.NewFlagSet("artifacts list", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the artifacts as JSON, with the path of the stored file")
	origin := fs.String("origin", "", "only artifacts whose parent job is ID")
	sinceArg := fs.String("since", "", "only artifacts published after an RFC3339 time or within a duration")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	var since time.Time
	if *sinceArg != "" {
		var err error
		if since, err = parseSince(*sinceArg, now); err != nil {
			return err
		}
	}

	arts, err := st.Artifacts()
	if err != nil {
		return err
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		return err
	}
	runs, err := st.Runs()
	if err != nil {
		return err
	}
	state, err := st.LoadState()
	if err != nil {
		return err
	}
	app.ResolveArtifactSources(arts, cfg.Tasks, runs)

	out := make([]listedArtifact, 0, len(arts))
	for _, a := range arts {
		if *origin != "" && a.OriginID != *origin {
			continue
		}
		if !since.IsZero() && !a.PublishedAt.After(since) {
			continue
		}
		out = append(out, listedArtifact{
			ID:          a.ID,
			Title:       a.Title,
			Description: a.Description,
			FileName:    a.FileName,
			Path:        st.ArtifactContentPath(a),
			Size:        a.Size,
			ContentType: a.ContentType,
			TaskID:      a.TaskID,
			TaskName:    a.TaskName,
			RunID:       a.RunID,
			Group:       a.Group,
			OriginID:    a.OriginID,
			OriginName:  a.OriginName,
			PublishedAt: a.PublishedAt,
			Unread:      !state.IsArtifactRead(a.ID),
		})
	}
	if *asJSON {
		return printJSON(out)
	}
	if len(out) == 0 {
		fmt.Println("no artifacts")
		return nil
	}

	unread := 0
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "\tID\tTITLE\tPARENT\tPUBLISHED")
	for _, a := range out {
		mark := " "
		if a.Unread {
			mark = "*"
			unread++
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			mark, a.ID, truncate(a.Title, 40), a.OriginName, a.PublishedAt.Local().Format("2006-01-02 15:04"))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("\n%d unread\n", unread)
	return nil
}

// parseSince reads --since: an RFC3339 time, or a positive duration counted
// back from now.
func parseSince(s string, now time.Time) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil || d <= 0 {
		return time.Time{}, fmt.Errorf("invalid --since %q (want an RFC3339 time or a duration such as 24h)", s)
	}
	return now.Add(-d), nil
}

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
)

// pager says where a list response keeps its rows and how large a page
// --all asks for when --limit was not given.
type pager struct {
	rowsKey  string // data.<rowsKey> holds the rows (records, legacy metadata)
	pageSize int
}

// runAll follows pageInfo.endCursor and writes ONE JSON array of the merged
// rows. It is the only place the CLI reshapes a response, so each row reaches
// the output exactly as Twenty sent it. Stopping early is never silent: the
// rows so far are written and the run ends as incomplete (the page cap),
// server (a page that claims more but gives no cursor), or with the error of
// a page after the first.
func (a *app) runAll(cmd *cobra.Command, base api.Request, p pager, out *outputFile) error {
	maxPages, _ := cmd.Flags().GetInt("max-pages")
	if maxPages < 1 {
		return api.Usagef("--max-pages must be at least 1, got %d", maxPages)
	}
	if base.Query.Get("ending_before") != "" {
		return api.Usagef("--all walks forward from the first page or from --starting-after; it cannot be combined with --ending-before")
	}
	q := cloneValues(base.Query)
	if q.Get("limit") == "" {
		q.Set("limit", strconv.Itoa(p.pageSize))
	}
	var rows []json.RawMessage
	var stop error
	complete := false
	for page := 0; page < maxPages && stop == nil && !complete; page++ {
		req := base
		req.Query = cloneValues(q)
		resp, err := a.client.Do(cmd.Context(), req)
		if err != nil {
			if page == 0 {
				return err
			}
			stop = err // a later page failed: keep what the earlier ones gave
			break
		}
		pageRows, cursor, more, err := parsePage(resp.Body, p.rowsKey)
		if err != nil {
			stop = &api.Error{Kind: api.KindServer, Message: fmt.Sprintf("%s %s: %v", req.Method, req.Path, err)}
			break
		}
		rows = append(rows, pageRows...)
		switch {
		case !more:
			complete = true
		case cursor == "":
			stop = &api.Error{Kind: api.KindServer, Message: fmt.Sprintf(
				"%s %s: pageInfo.hasNextPage is true but endCursor is empty; stopped instead of fetching the first page again, the output is partial",
				req.Method, req.Path)}
		default:
			q.Set("starting_after", cursor)
		}
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, r := range rows {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(r)
	}
	buf.WriteString("]\n")
	if err := a.writeResponse(&api.Response{Body: buf.Bytes()}, out); err != nil {
		return err
	}
	if stop != nil {
		return stop
	}
	if !complete {
		return &api.Error{Kind: api.KindIncomplete, Message: "hit --max-pages before the last page; the output is partial"}
	}
	return nil
}

// parsePage reads one list response: rows from data.<rowsKey> or, in the
// newer metadata format, from data itself; the cursor from pageInfo.
func parsePage(body []byte, rowsKey string) (rows []json.RawMessage, endCursor string, hasNext bool, err error) {
	var env struct {
		Data     json.RawMessage `json:"data"`
		PageInfo *struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, "", false, fmt.Errorf("--all expects a JSON object response: %v", err)
	}
	var inner map[string]json.RawMessage
	if json.Unmarshal(env.Data, &inner) == nil && inner != nil {
		raw, ok := inner[rowsKey]
		if !ok || json.Unmarshal(raw, &rows) != nil {
			return nil, "", false, fmt.Errorf("--all expected data.%s to be an array", rowsKey)
		}
	} else if json.Unmarshal(env.Data, &rows) != nil || env.Data == nil {
		return nil, "", false, fmt.Errorf("--all expected data.%s or data to be an array, got %.80s", rowsKey, env.Data)
	}
	if env.PageInfo == nil {
		return rows, "", false, nil
	}
	return rows, env.PageInfo.EndCursor, env.PageInfo.HasNextPage, nil
}

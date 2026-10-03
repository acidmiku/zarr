package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCancelSABJobOutsideDownloadQueue(t *testing.T) {
	for _, status := range []string{"", "Completed", "Failed", "Extracting", "Queued"} {
		t.Run("history_"+status, func(t *testing.T) {
			var operations []string
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				operation := q.Get("mode") + "/" + q.Get("name")
				operations = append(operations, operation)
				switch operation {
				case "queue/delete":
					fmt.Fprint(w, `{"status":false,"nzo_ids":[]}`)
				case "queue/", "history/":
					if q.Get("nzo_ids") != "job-1" || q.Get("limit") != "1" || q.Get("start") != "0" {
						t.Error("lookup must target the exact job, independently of history size")
					}
					if operation == "history/" && status != "" {
						fmt.Fprintf(w, `{"history":{"slots":[{"nzo_id":"job-1","status":%q}],"noofslots":1}}`, status)
					} else {
						fmt.Fprintf(w, `{%q:{"slots":[],"noofslots":0}}`, q.Get("mode"))
					}
				case "cancel_pp/", "history/delete":
					if q.Get("value") != "job-1" {
						t.Error("cancelled the wrong post-processing job")
					}
					if operation == "history/delete" && q.Get("del_files") != "0" {
						t.Error("cancellation must preserve completed files")
					}
					fmt.Fprint(w, `{"status":true}`)
				default:
					t.Errorf("unexpected operation %s", operation)
					w.WriteHeader(500)
				}
			}))
			defer remote.Close()
			g := New(remote.Client(), remote.Client(), remote.URL, "secret")
			if err := g.DeleteFromQueue("job-1"); err != nil {
				t.Fatal(err)
			}
			want := "[queue/delete queue/ history/]"
			if status == "Extracting" || status == "Queued" {
				want = "[queue/delete queue/ history/ cancel_pp/ history/delete]"
			}
			if fmt.Sprint(operations) != want {
				t.Fatalf("operations = %v, want %s", operations, want)
			}
		})
	}
}

func TestCancelSABJobDoesNotHideFailures(t *testing.T) {
	for _, tc := range []struct {
		name, operation, body string
		code                  int
		postProcessing        bool
	}{
		{"delete authentication", "queue/delete", `{"status":false,"error":"API Key Incorrect: secret"}`, 200, false},
		{"delete other rejection", "queue/delete", `{"status":false,"error":"Permission denied"}`, 200, false},
		{"delete unavailable", "queue/delete", `unavailable`, 503, false},
		{"delete invalid JSON", "queue/delete", `<html>error</html>`, 200, false},
		{"delete missing confirmation", "queue/delete", `{}`, 200, false},
		{"queue authentication", "queue/", `{"status":false,"error":"API Key Incorrect"}`, 200, false},
		{"queue missing", "queue/", `{}`, 200, false},
		{"queue missing slots", "queue/", `{"queue":{"noofslots":0}}`, 200, false},
		{"queue null slots", "queue/", `{"queue":{"slots":null,"noofslots":0}}`, 200, false},
		{"queue missing count", "queue/", `{"queue":{"slots":[]}}`, 200, false},
		{"queue truncated", "queue/", `{"queue":{"slots":[],"noofslots":1}}`, 200, false},
		{"queue still active", "queue/", `{"queue":{"slots":[{"nzo_id":"job-1","status":"Downloading"}],"noofslots":1}}`, 200, false},
		{"queue wrong ID", "queue/", `{"queue":{"slots":[{"nzo_id":"other-job","status":"Paused"}],"noofslots":1}}`, 200, false},
		{"history authentication", "history/", `{"status":false,"error":"API Key Incorrect"}`, 200, false},
		{"history unavailable", "history/", `unavailable`, 503, false},
		{"history missing", "history/", `{}`, 200, false},
		{"history truncated", "history/", `{"history":{"slots":[],"noofslots":20}}`, 200, false},
		{"history wrong ID", "history/", `{"history":{"slots":[{"nzo_id":"other-job","status":"Completed"}],"noofslots":1}}`, 200, false},
		{"history missing status", "history/", `{"history":{"slots":[{"nzo_id":"job-1"}],"noofslots":1}}`, 200, false},
		{"abort rejected", "cancel_pp/", `{"status":false,"error":"No such item"}`, 200, true},
		{"history delete unconfirmed", "history/delete", `{"status":false}`, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failed := false
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				operation := r.URL.Query().Get("mode") + "/" + r.URL.Query().Get("name")
				if failed {
					t.Error("continued after a real error")
				}
				if operation == tc.operation {
					failed = true
					w.WriteHeader(tc.code)
					fmt.Fprint(w, tc.body)
					return
				}
				switch operation {
				case "queue/delete":
					fmt.Fprint(w, `{"status":false,"nzo_ids":[]}`)
				case "queue/":
					fmt.Fprint(w, `{"queue":{"slots":[],"noofslots":0}}`)
				case "history/":
					if tc.postProcessing {
						fmt.Fprint(w, `{"history":{"slots":[{"nzo_id":"job-1","status":"Repairing"}],"noofslots":1}}`)
					} else {
						fmt.Fprint(w, `{"history":{"slots":[],"noofslots":0}}`)
					}
				case "cancel_pp/":
					fmt.Fprint(w, `{"status":true}`)
				default:
					t.Errorf("unexpected operation %s", operation)
					w.WriteHeader(500)
				}
			}))
			defer remote.Close()
			g := New(remote.Client(), remote.Client(), remote.URL, "secret")
			if err := g.DeleteFromQueue("job-1"); err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("expected a safe cancellation failure, got %v", err)
			}
			if !failed {
				t.Fatal("did not exercise the expected failure")
			}
		})
	}
}

func TestCancelSABRejectsBulkJobIDs(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid job ID reached SABnzbd")
	}))
	defer remote.Close()
	g := New(remote.Client(), remote.Client(), remote.URL, "secret")
	for _, id := range []string{"", "ALL", "failed", "completed", "job-1,job-2", " all "} {
		if err := g.DeleteFromQueue(id); err == nil {
			t.Errorf("accepted unsafe job ID %q", id)
		}
	}
}

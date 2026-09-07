package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/go-libs/v2/service"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

func TestKubernetesWatchStreamsWithDebug(t *testing.T) {
	// Always use the synthetic kubeconfig, even when CI runs inside Kubernetes.
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	for _, debug := range []bool{false, true} {
		for _, initialEvents := range []bool{false, true} {
			t.Run(fmt.Sprintf("debug=%t/initialEvents=%t", debug, initialEvents), func(t *testing.T) {
				streamClosed := make(chan struct{})
				flushed := make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/apis/formance.com/v1beta1/versions" || r.URL.Query().Get("watch") != "true" {
						http.Error(w, "expected a Versions watch", http.StatusBadRequest)
						return
					}
					if initialEvents && r.URL.Query().Get("sendInitialEvents") != "true" {
						http.Error(w, "expected initial watch events", http.StatusBadRequest)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprintln(w, `{"type":"ADDED","object":{"apiVersion":"formance.com/v1beta1","kind":"Versions","metadata":{"name":"test-version","resourceVersion":"1"}}}`)
					if initialEvents {
						_, _ = fmt.Fprintln(w, `{"type":"BOOKMARK","object":{"apiVersion":"formance.com/v1beta1","kind":"Versions","metadata":{"resourceVersion":"1","annotations":{"k8s.io/initial-events-end":"true"}}}}`)
					}
					w.(http.Flusher).Flush()
					close(flushed)
					// EOF must only be sent after the client has consumed the events.
					<-streamClosed
				}))
				defer server.Close()

				kubeConfigPath := filepath.Join(t.TempDir(), "kubeconfig")
				require.NoError(t, os.WriteFile(kubeConfigPath, fmt.Appendf(nil, `apiVersion: v1
kind: Config
current-context: test
contexts:
- name: test
  context:
    cluster: test
clusters:
- name: test
  cluster:
    server: %s
`, server.URL), 0600))
				cmd := &cobra.Command{}
				cmd.Flags().String(kubeConfigFlag, kubeConfigPath, "")
				cmd.Flags().Bool(service.DebugFlag, debug, "")
				config, err := createK8SConfig(cmd)
				require.NoError(t, err)
				client, err := dynamic.NewForConfig(config)
				require.NoError(t, err)

				ctx, cancel := context.WithTimeout(logging.ContextWithLogger(context.Background(),
					logging.NewDefaultLogger(io.Discard, debug, false, false)), 10*time.Second)
				defer cancel()
				events := make(chan watch.Event, 2)
				errors := make(chan error, 1)
				done := make(chan struct{})
				go func() {
					defer close(done)
					options := metav1.ListOptions{}
					if initialEvents {
						options.SendInitialEvents = &initialEvents
						options.AllowWatchBookmarks = true
						options.ResourceVersionMatch = metav1.ResourceVersionMatchNotOlderThan
					}
					stream, err := client.Resource(schema.GroupVersionResource{
						Group: "formance.com", Version: "v1beta1", Resource: "versions",
					}).Watch(ctx, options)
					if err != nil {
						errors <- err
						return
					}
					defer stream.Stop()
					count := 1
					if initialEvents {
						count++
					}
					for range count {
						events <- <-stream.ResultChan()
					}
				}()
				defer func() {
					close(streamClosed)
					<-done
				}()

				select {
				case <-flushed:
				case err := <-errors:
					t.Fatal(err)
				case <-time.After(5 * time.Second):
					t.Fatal("watch request did not reach the server")
				}
				expected := []watch.EventType{watch.Added}
				if initialEvents {
					expected = append(expected, watch.Bookmark)
				}
				for _, eventType := range expected {
					select {
					case event := <-events:
						require.Equal(t, eventType, event.Type)
						require.NotNil(t, event.Object)
					case err := <-errors:
						t.Fatal(err)
					case <-time.After(time.Second):
						t.Fatal("watch event was not delivered while the response body remained open")
					}
				}
			})
		}
	}
}

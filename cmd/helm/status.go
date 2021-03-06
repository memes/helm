/*
Copyright The Helm Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"io"
	"regexp"
	"text/tabwriter"

	"github.com/gosuri/uitable"
	"github.com/gosuri/uitable/util/strutil"
	"github.com/spf13/cobra"

	"k8s.io/helm/pkg/helm"
	"k8s.io/helm/pkg/proto/hapi/release"
	"k8s.io/helm/pkg/proto/hapi/services"
	"k8s.io/helm/pkg/timeconv"
)

var statusHelp = `
This command shows the status of a named release.
The status consists of:
- last deployment time
- k8s namespace in which the release lives
- state of the release (can be: UNKNOWN, DEPLOYED, DELETED, SUPERSEDED, FAILED or DELETING)
- list of resources that this release consists of, sorted by kind
- details on last test suite run, if applicable
- additional notes provided by the chart
`

type statusCmd struct {
	release string
	out     io.Writer
	client  helm.Interface
	version int32
	outfmt  string
}

func newStatusCmd(client helm.Interface, out io.Writer) *cobra.Command {
	status := &statusCmd{
		out:    out,
		client: client,
	}

	cmd := &cobra.Command{
		Use:     "status [flags] RELEASE_NAME",
		Short:   "Displays the status of the named release",
		Long:    statusHelp,
		PreRunE: func(_ *cobra.Command, _ []string) error { return setupConnection() },
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errReleaseRequired
			}
			status.release = args[0]
			if status.client == nil {
				status.client = newClient()
			}
			return status.run()
		},
	}

	f := cmd.Flags()
	settings.AddFlagsTLS(f)
	f.Int32Var(&status.version, "revision", 0, "If set, display the status of the named release with revision")
	bindOutputFlag(cmd, &status.outfmt)

	// set defaults from environment
	settings.InitTLS(f)

	return cmd
}

func (s *statusCmd) run() error {
	res, err := s.client.ReleaseStatus(s.release, helm.StatusReleaseVersion(s.version))
	if err != nil {
		return prettyError(err)
	}

	return write(s.out, &statusWriter{res}, outputFormat(s.outfmt))
}

type statusWriter struct {
	status *services.GetReleaseStatusResponse
}

func (s *statusWriter) WriteTable(out io.Writer) error {
	PrintStatus(out, s.status)
	// There is no error handling here due to backwards compatibility with
	// PrintStatus
	return nil
}

func (s *statusWriter) WriteJSON(out io.Writer) error {
	return encodeJSON(out, s.status)
}

func (s *statusWriter) WriteYAML(out io.Writer) error {
	return encodeYAML(out, s.status)
}

// PrintStatus prints out the status of a release. Shared because also used by
// install / upgrade
func PrintStatus(out io.Writer, res *services.GetReleaseStatusResponse) {
	if res.Info.LastDeployed != nil {
		fmt.Fprintf(out, "LAST DEPLOYED: %s\n", timeconv.String(res.Info.LastDeployed))
	}
	fmt.Fprintf(out, "NAMESPACE: %s\n", res.Namespace)
	fmt.Fprintf(out, "STATUS: %s\n", res.Info.Status.Code)
	fmt.Fprintf(out, "\n")
	if len(res.Info.Status.Resources) > 0 {
		re := regexp.MustCompile("  +")

		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.TabIndent)
		fmt.Fprintf(w, "RESOURCES:\n%s\n", re.ReplaceAllString(res.Info.Status.Resources, "\t"))
		w.Flush()
	}
	if res.Info.Status.LastTestSuiteRun != nil {
		lastRun := res.Info.Status.LastTestSuiteRun
		fmt.Fprintf(out, "TEST SUITE:\n%s\n%s\n\n%s\n",
			fmt.Sprintf("Last Started: %s", timeconv.String(lastRun.StartedAt)),
			fmt.Sprintf("Last Completed: %s", timeconv.String(lastRun.CompletedAt)),
			formatTestResults(lastRun.Results))
	}

	if len(res.Info.Status.Notes) > 0 {
		fmt.Fprintf(out, "NOTES:\n%s\n", res.Info.Status.Notes)
	}
}

func formatTestResults(results []*release.TestRun) string {
	tbl := uitable.New()
	tbl.MaxColWidth = 50
	tbl.AddRow("TEST", "STATUS", "INFO", "STARTED", "COMPLETED")
	for i := 0; i < len(results); i++ {
		r := results[i]
		n := r.Name
		s := strutil.PadRight(r.Status.String(), 10, ' ')
		i := r.Info
		ts := timeconv.String(r.StartedAt)
		tc := timeconv.String(r.CompletedAt)
		tbl.AddRow(n, s, i, ts, tc)
	}
	return tbl.String()
}

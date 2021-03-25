package run

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/workflow/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type RunOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	Selector string
	Ref      string

	Prompt bool
}

func NewCmdRun(f *cmdutil.Factory, runF func(*RunOptions) error) *cobra.Command {
	opts := &RunOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:    "run [<workflow ID> | <workflow name>]",
		Short:  "Create a dispatch event for a workflow, starting a run",
		Args:   cobra.MaximumNArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.Selector = args[0]
			} else if !opts.IO.CanPrompt() {
				return &cmdutil.FlagError{Err: errors.New("workflow ID or name required when not running interactively")}
			} else {
				opts.Prompt = true
			}

			if runF != nil {
				return runF(opts)
			}
			return runRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Ref, "ref", "r", "", "The branch or tag name which contains the version of the workflow file you'd like to run")

	return cmd
}

func runRun(opts *RunOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	states := []shared.WorkflowState{shared.Active}
	workflow, err := shared.ResolveWorkflow(
		opts.IO, client, repo, opts.Prompt, opts.Selector, states)
	if err != nil {
		var fae shared.FilteredAllError
		if errors.As(err, &fae) {
			return errors.New("no workflows are enabled on this repository")
		}
		return err
	}

	ref := opts.Ref

	if ref == "" {
		ref, err = api.RepoDefaultBranch(client, repo)
		if err != nil {
			return fmt.Errorf("unable to determine default branch for %s: %w", ghrepo.FullName(repo), err)
		}
	}

	fmt.Printf("DBG %#v\n", workflow)
	fmt.Printf("DBG %#v\n", ref)

	yamlContent, err := getWorkflowContent(client, repo, workflow, ref)
	if err != nil {
		return fmt.Errorf("unable to fetch workflow file content: %w", err)
	}

	fmt.Printf("DBG %#v\n", yamlContent)

	// TODO  once end-to-end is working, circle back and see if running a local workflow remotely is feasible by doing git stuff automagically in a throwaway branch.

	// TODO get yaml
	// TODO parse yaml
	// TODO ensure it is a workflow_dispatch
	// TODO generate survey prompts for the inputs
	// TODO validate whatever input we got
	// TODO create the dispatch event

	return nil
}

func getWorkflowContent(client *api.Client, repo ghrepo.Interface, workflow *shared.Workflow, ref string) (string, error) {
	path := fmt.Sprintf("repos/%s/contents/%s?ref=%s", ghrepo.FullName(repo), workflow.Path, url.QueryEscape(ref))

	type Result struct {
		Content string
	}

	var result Result
	err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
	if err != nil {
		return "", err
	}

	decoded, err := base64.StdEncoding.DecodeString(result.Content)
	if err != nil {
		return "", fmt.Errorf("failed to decode workflow file: %w", err)
	}

	return string(decoded), nil
}

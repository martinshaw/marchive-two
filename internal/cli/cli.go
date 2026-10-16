package cli

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/martinshaw/marchive-two/internal/app"
	"github.com/martinshaw/marchive-two/internal/browser"
	"github.com/martinshaw/marchive-two/internal/client"
	"github.com/martinshaw/marchive-two/internal/lock"
	"github.com/martinshaw/marchive-two/internal/paths"
	"github.com/martinshaw/marchive-two/internal/service"
	"github.com/martinshaw/marchive-two/internal/store"
	"github.com/martinshaw/marchive-two/internal/version"
)

var dataDirFlag string

// Execute runs the marchive command.
func Execute() error {
	root := &cobra.Command{
		Use:           "marchive",
		Short:         "Archive webpages, articles, and podcasts on a schedule",
		Version:       version.Version,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd, true)
		},
	}
	root.PersistentFlags().StringVar(&dataDirFlag, "data-dir", "", "data directory (default: ~/Library/Application Support/Marchive)")

	root.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Run the scheduler and UI without opening a browser",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd, false)
		},
	})

	sources := &cobra.Command{Use: "sources", Short: "List and edit sources"}
	sources.AddCommand(sourcesList(), sourcesAdd(), sourcesDelete())
	root.AddCommand(sources)

	schedules := &cobra.Command{Use: "schedules", Short: "Run or edit schedules"}
	schedules.AddCommand(schedulesList(), schedulesRun(), schedulesSet())
	root.AddCommand(schedules)

	runnerCmd := &cobra.Command{Use: "runner", Short: "Start or stop scheduled captures"}
	runnerCmd.AddCommand(runnerStart(), runnerStop(), runnerStatus())
	root.AddCommand(runnerCmd)

	runs := &cobra.Command{Use: "runs", Short: "Inspect captures"}
	runs.AddCommand(runsList(), runsCancel())
	root.AddCommand(runs)

	root.AddCommand(browserCmd())

	return root.Execute()
}

func browserCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "browser", Short: "Manage the capture browser"}
	cmd.AddCommand(&cobra.Command{
		Use:   "login",
		Short: "Open Chrome so you can sign in to sites you subscribe to",
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, err := ensureServer()
			if err != nil {
				return err
			}
			if err := client.New(base).SignIn(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Chrome is opening. Sign in, then quit Chrome.")
			return nil
		},
	})
	return cmd
}

func serve(cmd *cobra.Command, openBrowser bool) error {
	dir, err := dataDir()
	if err != nil {
		return err
	}
	if base, ok := app.Probe(dir); ok {
		fmt.Fprintf(cmd.OutOrStdout(), "Marchive is already running at %s\n", base)
		if openBrowser {
			return app.Open(base + "/")
		}
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), stopSignals()...)
	defer stop()
	return app.Serve(ctx, app.Options{DataDir: dir, OpenBrowser: openBrowser})
}

func sourcesList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List sources and schedules",
		RunE: func(cmd *cobra.Command, _ []string) error {
			state, err := currentState()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SCHEDULE\tSOURCE\tKIND\tEVERY\tENABLED\tURL")
			for _, src := range state.Sources {
				if len(src.Schedules) == 0 {
					fmt.Fprintf(w, "-\t%s\t%s\t-\t-\t%s\n", src.Name, src.Provider, src.URL)
				}
				for _, sch := range src.Schedules {
					fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%t\t%s\n", sch.ID, src.Name, src.Provider, everyLabel(sch.IntervalSeconds), sch.Enabled, src.URL)
				}
			}
			return w.Flush()
		},
	}
}

func sourcesAdd() *cobra.Command {
	var name, rawURL, kind, selector, every, dest string
	var maxNew int
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a source and a schedule",
		RunE: func(cmd *cobra.Command, _ []string) error {
			seconds, err := durationSeconds(every)
			if err != nil {
				return err
			}
			enabled := true
			in := service.SourceInput{
				Name:             name,
				URL:              rawURL,
				Provider:         kind,
				LinkSelector:     selector,
				IntervalSeconds:  seconds,
				DownloadLocation: dest,
				MaxNew:           maxNew,
				Enabled:          &enabled,
			}
			return withEither(func(c *client.Client) error {
				id, scheduleID, err := c.CreateSource(in)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Added source %d with schedule %d\n", id, scheduleID)
				return nil
			}, func(svc *service.Service) error {
				id, scheduleID, err := svc.CreateSource(in)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Added source %d with schedule %d\n", id, scheduleID)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "display name")
	cmd.Flags().StringVar(&rawURL, "url", "", "page, feed, or section URL")
	cmd.Flags().StringVar(&kind, "provider", "", "webpage, article_index, podcast, or text_extract")
	cmd.Flags().StringVar(&selector, "selector", "", "CSS selector for article links")
	cmd.Flags().StringVar(&every, "every", "1h", "how often to check, such as 30m or 6h")
	cmd.Flags().StringVar(&dest, "dest", "", "folder for archived files")
	cmd.Flags().IntVar(&maxNew, "max-new", 20, "new articles or episodes to take per run (0 is unlimited)")
	_ = cmd.MarkFlagRequired("url")
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}

func sourcesDelete() *cobra.Command {
	return &cobra.Command{
		Use:   "delete ID",
		Short: "Delete a source and its schedules",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			return withEither(func(c *client.Client) error {
				return c.DeleteSource(id)
			}, func(svc *service.Service) error {
				return svc.DeleteSource(id)
			})
		},
	}
}

func schedulesList() *cobra.Command {
	return sourcesList()
}

func schedulesRun() *cobra.Command {
	return &cobra.Command{
		Use:   "run ID",
		Short: "Capture a schedule now",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			base, err := ensureServer()
			if err != nil {
				return err
			}
			runID, err := client.New(base).RunNow(id)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Queued capture %d\n", runID)
			return nil
		},
	}
}

func schedulesSet() *cobra.Command {
	var every, dest, enabled string
	var maxNew int
	cmd := &cobra.Command{
		Use:   "set ID",
		Short: "Change a schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			var in service.ScheduleInput
			if cmd.Flags().Changed("every") {
				seconds, err := durationSeconds(every)
				if err != nil {
					return err
				}
				in.IntervalSeconds = &seconds
			}
			if cmd.Flags().Changed("dest") {
				in.DownloadLocation = &dest
			}
			if cmd.Flags().Changed("max-new") {
				in.MaxNew = &maxNew
			}
			if cmd.Flags().Changed("enabled") {
				on, err := strconv.ParseBool(enabled)
				if err != nil {
					return fmt.Errorf("--enabled must be true or false")
				}
				in.Enabled = &on
			}
			return withEither(func(c *client.Client) error {
				return c.UpdateSchedule(id, in)
			}, func(svc *service.Service) error {
				return svc.UpdateSchedule(id, in)
			})
		},
	}
	cmd.Flags().StringVar(&every, "every", "", "interval such as 30m or 6h")
	cmd.Flags().StringVar(&dest, "dest", "", "archive folder")
	cmd.Flags().IntVar(&maxNew, "max-new", 20, "new items per run")
	cmd.Flags().StringVar(&enabled, "enabled", "", "true or false")
	return cmd
}

func runnerStart() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start scheduled captures",
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, err := ensureServer()
			if err != nil {
				return err
			}
			if err := client.New(base).SetRunner(true); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Runner started.")
			return nil
		},
	}
}

func runnerStop() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop scheduling and cancel queued captures",
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := withEither(func(c *client.Client) error {
				return c.SetRunner(false)
			}, func(svc *service.Service) error {
				return svc.SetRunner(false)
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Runner stopped.")
			return nil
		},
	}
}

func runnerStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the runner is on",
		RunE: func(cmd *cobra.Command, _ []string) error {
			state, err := currentState()
			if err != nil {
				return err
			}
			if state.RunnerEnabled {
				fmt.Fprintln(cmd.OutOrStdout(), "Runner is on.")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Runner is off.")
			}
			return nil
		},
	}
}

func runsList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List recent captures",
		RunE: func(cmd *cobra.Command, _ []string) error {
			state, err := currentState()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "RUN\tSTATUS\tSOURCE\tNOTE")
			for _, run := range state.Runs {
				note := run.Note
				if run.Error != "" {
					note = run.Error
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", run.ID, run.Status, run.SourceName, note)
			}
			return w.Flush()
		},
	}
}

func runsCancel() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel ID",
		Short: "Cancel a queued or running capture",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			base, err := ensureServer()
			if err != nil {
				return err
			}
			return client.New(base).CancelRun(id)
		},
	}
}

func withEither(remote func(*client.Client) error, local func(*service.Service) error) error {
	dir, err := dataDir()
	if err != nil {
		return err
	}
	if base, ok := app.Probe(dir); ok {
		return remote(client.New(base))
	}
	svc, done, err := openLocal(dir)
	if err != nil {
		return err
	}
	defer done()
	return local(svc)
}

func currentState() (service.State, error) {
	var state service.State
	err := withEither(func(c *client.Client) error {
		var err error
		state, err = c.State()
		return err
	}, func(svc *service.Service) error {
		var err error
		state, err = svc.State()
		return err
	})
	return state, err
}

func ensureServer() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	if base, ok := app.Probe(dir); ok {
		return base, nil
	}
	if err := app.StartBackground(dir); err != nil {
		return "", err
	}
	return app.WaitReady(dir, 15*time.Second)
}

func openLocal(dir string) (*service.Service, func(), error) {
	lk, err := lock.Acquire(paths.Lock(dir))
	if errors.Is(err, lock.ErrBusy) {
		return nil, nil, errors.New("Marchive is running but not answering yet. Try again in a moment.")
	}
	if err != nil {
		return nil, nil, err
	}
	st, err := store.Open(paths.DB(dir))
	if err != nil {
		lk.Release()
		return nil, nil, err
	}
	svc := service.New(st, nil, browser.New(paths.Profile(dir)))
	return svc, func() {
		st.Close()
		lk.Release()
	}, nil
}

func dataDir() (string, error) {
	return paths.Resolve(dataDirFlag)
}

func parseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid id %q", raw)
	}
	return id, nil
}

func durationSeconds(raw string) (int, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("interval must look like 30m or 1h")
	}
	if d < 30*time.Second {
		return 0, fmt.Errorf("the interval must be at least 30 seconds")
	}
	return int(d.Seconds()), nil
}

func everyLabel(seconds int) string {
	if seconds%3600 == 0 {
		return fmt.Sprintf("%dh", seconds/3600)
	}
	if seconds%60 == 0 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%ds", seconds)
}

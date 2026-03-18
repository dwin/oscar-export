package cmd

import (
	"fmt"
	"time"

	exporter "github.com/dwin/oscar-export/internal/export"
	"github.com/spf13/cobra"
)

type exportOptions struct {
	root        string
	profileUser string
	out         string
	serial      string
	from        string
	to          string
}

func init() {
	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export OSCAR cache data to CSV",
	}

	exportCmd.AddCommand(newExportModeCommand(exporter.ModeSummary, "summary", "Export day-level summary CSV"))
	exportCmd.AddCommand(newExportModeCommand(exporter.ModeSessions, "sessions", "Export per-session CSV"))
	exportCmd.AddCommand(newExportModeCommand(exporter.ModeDetails, "details", "Export detail rows CSV"))

	rootCmd.AddCommand(exportCmd)
}

func newExportModeCommand(mode exporter.Mode, use, short string) *cobra.Command {
	opts := &exportOptions{}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			from, err := time.Parse("2006-01-02", opts.from)
			if err != nil {
				return fmt.Errorf("parse --from: %w", err)
			}
			to, err := time.Parse("2006-01-02", opts.to)
			if err != nil {
				return fmt.Errorf("parse --to: %w", err)
			}
			if to.Before(from) {
				return fmt.Errorf("--to must be on or after --from")
			}

			cfg := exporter.Config{
				Mode:        mode,
				Root:        opts.root,
				ProfileUser: opts.profileUser,
				Serial:      opts.serial,
				Out:         opts.out,
				From:        from,
				To:          to,
			}
			return exporter.Run(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&opts.root, "root", "", "Path to the OSCAR data root")
	cmd.Flags().StringVar(&opts.profileUser, "profile-user", "", "OSCAR profile user name")
	cmd.Flags().StringVar(&opts.serial, "serial", "", "Optional machine serial to export")
	cmd.Flags().StringVar(&opts.from, "from", "", "Start sleep date in YYYY-MM-DD")
	cmd.Flags().StringVar(&opts.to, "to", "", "End sleep date in YYYY-MM-DD")
	cmd.Flags().StringVar(&opts.out, "out", "", "Output CSV path (optional)")

	_ = cmd.MarkFlagRequired("root")
	_ = cmd.MarkFlagRequired("profile-user")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")

	return cmd
}

// Command channel-plugin-maintain manages explicitly selected archived programs
// and abandoned pending captures while ALL gateway instances are stopped. It
// does not run the server, migrate a schema or discover credential material.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

type selections []string

func (s *selections) String() string         { return "" }
func (s *selections) Set(value string) error { *s = append(*s, value); return nil }

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv); err != nil {
		// Do not print raw driver/compiler errors, DSNs, source or identities.
		fmt.Fprintln(os.Stderr, publicError(err))
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out, diagnostics io.Writer, lookup func(string) (string, bool)) error {
	flags := flag.NewFlagSet("channel-plugin-maintain", flag.ContinueOnError)
	// flag.Parse can echo an arbitrary malformed argument, including a DSN
	// accidentally passed by the caller. Keep parser diagnostics private.
	flags.SetOutput(io.Discard)
	mode := flags.String("mode", "check", "inspect, check or apply (default check)")
	dialect := flags.String("dialect", "", "sqlite or postgres")
	dsnEnv := flags.String("dsn-env", "", "Name of a caller-scoped environment variable containing the database DSN (never a DSN argument)")
	planPath := flags.String("plan", "", "Private JSON plan file for check/apply")
	offline := flags.Bool("ack-all-instances-stopped", false, "Confirm ALL gateway instances, including old hosts, are stopped; a database lock cannot prove this")
	var archives, captures selections
	flags.Var(&archives, "archive", "Exact plugin key:fingerprint to inspect (repeatable, inspect only)")
	flags.Var(&captures, "capture", "Exact pending public ID to inspect (repeatable, inspect only)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(diagnostics)
			flags.PrintDefaults()
			return nil
		}
		return channelplugins.ErrMaintenancePlan
	}
	if flags.NArg() != 0 || (*dialect != store.DialectSQLite && *dialect != store.DialectPostgres) || !validEnvName(*dsnEnv) {
		return channelplugins.ErrMaintenancePlan
	}
	if *mode != "inspect" && *mode != "check" && *mode != "apply" {
		return channelplugins.ErrMaintenancePlan
	}
	if *mode == "apply" && !*offline {
		return channelplugins.ErrMaintenanceOffline
	}
	var plan channelplugins.MaintenancePlan
	if *mode == "inspect" {
		if *planPath != "" || len(archives)+len(captures) == 0 || len(archives) > 64 || len(captures) > 64 {
			return channelplugins.ErrMaintenancePlan
		}
		for _, selected := range archives {
			key, fingerprint, ok := strings.Cut(selected, ":")
			if !ok {
				return channelplugins.ErrMaintenancePlan
			}
			plan.Archives = append(plan.Archives, channelplugins.MaintenanceVersion{Key: key, Fingerprint: fingerprint})
		}
		for _, id := range captures {
			plan.Captures = append(plan.Captures, channelplugins.MaintenanceCapture{ID: id, SnapshotSHA256: strings.Repeat("0", 64)})
		}
		if err := plan.Validate(); err != nil {
			return err
		}
	} else {
		if *planPath == "" || len(archives)+len(captures) != 0 {
			return channelplugins.ErrMaintenancePlan
		}
		var err error
		plan, err = readPlan(*planPath)
		if err != nil {
			return err
		}
	}
	dsn, ok := lookup(*dsnEnv)
	if !ok || dsn == "" {
		return errors.New("maintenance_database_unavailable")
	}
	if *dialect == store.DialectSQLite {
		// Avoid silently creating an empty DB or following an uncertain symlink.
		info, err := os.Lstat(dsn)
		if err != nil || !filepath.IsAbs(dsn) || !info.Mode().IsRegular() {
			return errors.New("maintenance_database_unavailable")
		}
	}
	db, err := store.Open(*dialect, dsn, false)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if *mode == "inspect" {
		plan, err = channelplugins.InspectMaintenancePlan(ctx, db.DB, plan.Archives, captures)
		if err != nil {
			return err
		}
		return encoder.Encode(plan)
	}
	result, err := channelplugins.MaintainArchivedVersions(ctx, db.DB, plan, *mode == "apply", *offline)
	if err != nil {
		return err
	}
	return encoder.Encode(result)
}

func readPlan(path string) (channelplugins.MaintenancePlan, error) {
	var plan channelplugins.MaintenancePlan
	file, err := os.Open(path)
	if err != nil {
		return plan, channelplugins.ErrMaintenancePlan
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return plan, channelplugins.ErrMaintenancePlan
	}
	raw, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 || requestoverride.RejectDuplicateJSONKeys(raw) != nil {
		return plan, channelplugins.ErrMaintenancePlan
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&plan) != nil || decoder.Decode(&extra) != io.EOF {
		return channelplugins.MaintenancePlan{}, channelplugins.ErrMaintenancePlan
	}
	return plan, plan.Validate()
}

func validEnvName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (index > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func publicError(err error) string {
	for _, expected := range []error{channelplugins.ErrMaintenancePlan, channelplugins.ErrMaintenanceOffline, channelplugins.ErrMaintenanceCapture, channelplugins.ErrVersionNotPrunable, channelplugins.ErrBinding, store.ErrChannelPluginReferences, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, expected) {
			return expected.Error()
		}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "channel_plugin_maintenance_archive_missing"
	}
	return "channel_plugin_maintenance_failed"
}

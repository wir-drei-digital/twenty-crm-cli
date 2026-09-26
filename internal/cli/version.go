package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Version is the released version, set with -ldflags at build time.
var Version = "dev"

// TestedTwentyVersion is the Twenty release this build was checked against.
const TestedTwentyVersion = "2.27"

// resolveVersion prefers the linker-set version. A build without one, such
// as `go install .../cmd/twentycrm@v0.1.0`, falls back to the module version
// Go recorded; "(devel)" stays "dev".
func resolveVersion(linker string, info *debug.BuildInfo, ok bool) string {
	if linker != "dev" || !ok || info == nil {
		return linker
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return linker
}

// commitOf returns the short VCS revision Go recorded, or "".
func commitOf(info *debug.BuildInfo, ok bool) string {
	if !ok || info == nil {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return s.Value[:7]
		}
	}
	return ""
}

func (a *app) versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version and the Twenty version it was tested against",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			info, ok := debug.ReadBuildInfo()
			commit := ""
			if c := commitOf(info, ok); c != "" {
				commit = "commit " + c + ", "
			}
			fmt.Fprintf(a.stdout, "twentycrm %s (%stested against Twenty %s)\n", resolveVersion(Version, info, ok), commit, TestedTwentyVersion)
		},
	}
}

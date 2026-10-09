package client

import (
	"testing"

	"github.com/fleetdm/fleet/v4/server/archtest"
)

const m = archtest.ModuleName

func TestClientPackageDoesNotImportServerService(t *testing.T) {
	t.Parallel()
	archtest.NewPackageTest(t, m+"/client...").
		ShouldNotDependOn(
			m+"/server/service...",
			m+"/ee/server/service...",
		).
		IgnoreDeps(
			m+"/server/service/externalsvc", // server/fleet has a dependency on Jira and Zendesk.
			// >>> OPENFRAME(agent-openframe-mode): the orbit client deliberately imports the
			// openframe auth manager (a leaf package: cron + zerolog only, no service layer) —
			// openframe/docs/agent-openframe-mode.md
			m+"/server/service/openframe",
			// <<< OPENFRAME(agent-openframe-mode)
		).
		Check()
}

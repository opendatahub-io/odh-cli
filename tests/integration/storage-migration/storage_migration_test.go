//go:build integration

package storagemigration_test

import (
	"testing"

	. "github.com/onsi/gomega"
)

// The first upgrade step, on a cluster that serves the upstream
// StorageVersionMigration API and runs its controller: v1 is the only version,
// then v2 becomes the storage version.
func TestMigrateV1ToV2(t *testing.T) {
	g := NewWithT(t)

	c := newCluster(t, withMigrationAPI)
	odh := newCLI(c)
	g.Expect(c.servesMigrationAPI(t)).To(BeTrue())

	t.Log("v1 is the only version: lint blocks, and there is nothing to migrate to")

	c.installCRD(t, "v1")
	c.createDSC(t, "v1")
	c.expectStoredVersions(t, "v1")
	odh.expectLintBlocks(t)

	odh.migrate(t)
	c.expectStoredVersions(t, "v1")
	odh.expectLintBlocks(t)

	t.Log("v2 becomes the storage version: v1 is still recorded, so lint blocks and v1 cannot be removed")

	c.installCRD(t, "v1", "v2")
	c.expectStoredVersions(t, "v1", "v2")
	odh.expectLintBlocks(t)
	c.expectCRDUpdateRejected(t, "v2")

	t.Log("a run without permissions and a dry run both leave the cluster untouched")

	before := c.resourceVersion(t, "v2")

	expectRestrictedUserCannotMigrate(t, c, odh)
	odh.migrate(t, "--dry-run")

	c.expectStoredVersions(t, "v1", "v2")
	g.Expect(c.resourceVersion(t, "v2")).To(Equal(before))

	t.Log("migrating rewrites the object, removes v1 from the history and unblocks lint")

	odh.migrate(t)

	g.Expect(c.resourceVersion(t, "v2")).ToNot(Equal(before))
	c.expectStoredVersions(t, "v2")
	c.expectDataPreserved(t, "v1", "v2")
	odh.expectLintPasses(t)
	expectMigratingAgainChangesNothing(t, c, odh, "v2")

	t.Log("v1 can now be removed from the CRD")

	c.installCRD(t, "v2")
	c.expectStoredVersions(t, "v2")
}

// The next upgrade step: v2 is the only version, then v3 becomes the storage
// version. The v1 lint check has nothing to report here, but the v2 objects
// still need rewriting.
//
// This runs on its own cluster. Continuing on the cluster of the test above,
// the upstream controller left the second migration request in Running and
// never finished it (observed for 4 minutes on Kubernetes 1.35).
func TestMigrateV2ToV3(t *testing.T) {
	g := NewWithT(t)

	c := newCluster(t, withMigrationAPI)
	odh := newCLI(c)
	g.Expect(c.servesMigrationAPI(t)).To(BeTrue())

	t.Log("v3 becomes the storage version: lint passes although v2 objects remain")

	c.installCRD(t, "v2")
	c.createDSC(t, "v2")
	c.installCRD(t, "v2", "v3")
	c.expectStoredVersions(t, "v2", "v3")
	odh.expectLintPasses(t)

	t.Log("migrating rewrites the object and removes v2 from the history")

	before := c.resourceVersion(t, "v3")

	odh.migrate(t)

	g.Expect(c.resourceVersion(t, "v3")).ToNot(Equal(before))
	c.expectStoredVersions(t, "v3")
	c.expectDataPreserved(t, "v2", "v3")
	odh.expectLintPasses(t)
	expectMigratingAgainChangesNothing(t, c, odh, "v3")
}

// The cluster serves no StorageVersionMigration API. The migration must refuse
// to run and leave the cluster untouched.
func TestMigrateWithoutMigrationAPI(t *testing.T) {
	g := NewWithT(t)

	c := newCluster(t, withoutMigrationAPI)
	odh := newCLI(c)
	g.Expect(c.servesMigrationAPI(t)).To(BeFalse())

	c.installCRD(t, "v1")
	c.createDSC(t, "v1")
	c.installCRD(t, "v1", "v2")

	before := c.resourceVersion(t, "v2")

	err := odh.migrateExpectingError(t)

	g.Expect(err).To(MatchError(ContainSubstring("enable the cluster storage migration API and controller")))
	g.Expect(c.resourceVersion(t, "v2")).To(Equal(before))
	c.expectStoredVersions(t, "v1", "v2")
	odh.expectLintBlocks(t)
}

// expectMigratingAgainChangesNothing runs the migration on an already migrated
// cluster and checks the object is not written again.
func expectMigratingAgainChangesNothing(t *testing.T, c *cluster, odh *cli, version string) {
	t.Helper()
	g := NewWithT(t)

	before := c.resourceVersion(t, version)

	odh.migrate(t)

	g.Expect(c.resourceVersion(t, version)).To(Equal(before))
}

// expectRestrictedUserCannotMigrate runs the migration as a user who may read
// the CRD and the DataScienceClusters but not create a migration request.
func expectRestrictedUserCannotMigrate(t *testing.T, c *cluster, odh *cli) {
	t.Helper()
	g := NewWithT(t)

	c.createRestrictedUser(t)

	err := odh.migrateExpectingError(t, "--as", restrictedUser)

	g.Expect(err).To(MatchError(ContainSubstring("forbidden")))
}

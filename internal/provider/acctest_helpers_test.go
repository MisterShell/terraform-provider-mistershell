package provider_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-mistershell/internal/client"
)

// acctestPrefix is the per-process unique prefix applied to every object name
// and external_id created by the acceptance suite. It isolates a run from
// reruns / parallel runs (MisterShell enforces unique names + external_ids) and
// gives the sweepers a precise target. Computed once at package init.
var acctestPrefix = fmt.Sprintf("tfacc-%06d-", rand.Intn(1000000))

// TestMain wires the terraform-plugin-testing sweeper framework so the -sweep
// flag is recognized. resource.TestMain also runs the normal test suite.
func TestMain(m *testing.M) {
	resource.TestMain(m)
}

// testAccClient builds a *client.Client from the MISTERSHELL_* environment
// variables, mirroring provider.go's configuration logic. Reused by the generic
// CheckDestroy and by the sweepers. Returns nil if the env is not configured.
func testAccClient() *client.Client {
	url := os.Getenv("MISTERSHELL_URL")
	apiKey := os.Getenv("MISTERSHELL_API_KEY")
	if url == "" || apiKey == "" {
		return nil
	}

	httpClient := &http.Client{}
	if v := os.Getenv("MISTERSHELL_INSECURE"); v == "1" || strings.EqualFold(v, "true") {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}

	return client.NewClient(url, apiKey, httpClient)
}

// testAccCheckAllDestroyed is the generic CheckDestroy: it iterates the
// post-destroy Terraform state and confirms each object is truly gone
// server-side, using the client Get* methods and client.IsNotFound. This is the
// only mechanism that actually proves deletion.
func testAccCheckAllDestroyed(s *terraform.State) error {
	c := testAccClient()
	if c == nil {
		return fmt.Errorf("testAccCheckAllDestroyed: MISTERSHELL_URL/MISTERSHELL_API_KEY not set")
	}

	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		id, perr := strconv.ParseInt(rs.Primary.ID, 10, 64)
		if perr != nil {
			continue
		}

		var err error
		switch rs.Type {
		case "mistershell_location":
			_, err = c.GetLocation(ctx, id)
		case "mistershell_resource":
			_, err = c.GetNetworkResource(ctx, id)
		case "mistershell_credential":
			_, err = c.GetCredential(ctx, id)
		default:
			continue
		}

		if err == nil {
			return fmt.Errorf("%s %d still exists after destroy", rs.Type, id)
		}
		if !client.IsNotFound(err) {
			return fmt.Errorf("unexpected error checking %s %d: %w", rs.Type, id, err)
		}
	}
	return nil
}

// discoverRootLocationID returns the id of the MisterShell root location: the
// lowest-id location with no parent. MisterShell auto-creates a single root and
// refuses to delete any root location (parent_id == null), so the suite's
// locations must hang beneath it to stay destroyable. Fails the test loudly if
// the env is unset or no root is found.
func discoverRootLocationID(t *testing.T) int64 {
	t.Helper()
	c := testAccClient()
	if c == nil {
		t.Fatal("discoverRootLocationID: MISTERSHELL_URL/MISTERSHELL_API_KEY not set")
	}
	locs, err := c.ListLocations(context.Background(), client.LocationListFilter{})
	if err != nil {
		t.Fatalf("discovering root location: %v", err)
	}
	rootID := int64(-1)
	for _, l := range locs {
		if l.ParentID == nil && (rootID == -1 || l.ID < rootID) {
			rootID = l.ID
		}
	}
	if rootID == -1 {
		t.Fatal("no root location (parent_id == null) found on the MisterShell instance")
	}
	return rootID
}

// ---------------------------------------------------------------------------
// Sweepers — orphan safety net for runs that crashed before destroy.
// Ordering matters: resources depend on credentials and locations, so resources
// must be swept first. terraform-plugin-testing honours the Dependencies field.
// ---------------------------------------------------------------------------

func init() {
	resource.AddTestSweepers("mistershell_resource", &resource.Sweeper{
		Name: "mistershell_resource",
		F:    sweepNetworkResources,
	})
	resource.AddTestSweepers("mistershell_credential", &resource.Sweeper{
		Name:         "mistershell_credential",
		Dependencies: []string{"mistershell_resource"},
		F:            sweepCredentials,
	})
	resource.AddTestSweepers("mistershell_location", &resource.Sweeper{
		Name:         "mistershell_location",
		Dependencies: []string{"mistershell_resource"},
		F:            sweepLocations,
	})
}

// sweepPrefix is the name prefix the sweepers match. It deliberately matches the
// "tfacc-" family (any run), not just the current process's acctestPrefix, so a
// sweep cleans up orphans from any previous crashed run.
const sweepPrefix = "tfacc-"

func sweepNetworkResources(_ string) error {
	c := testAccClient()
	if c == nil {
		return nil // not configured — skip gracefully
	}
	ctx := context.Background()
	items, err := c.ListNetworkResources(ctx, client.NetworkResourceListFilter{Search: sweepPrefix})
	if err != nil {
		return fmt.Errorf("sweep: listing network resources: %w", err)
	}
	for _, it := range items {
		if !strings.HasPrefix(it.Name, sweepPrefix) {
			continue
		}
		if err := c.DeleteNetworkResource(ctx, it.ID); err != nil && !client.IsNotFound(err) {
			return fmt.Errorf("sweep: deleting network resource %d (%s): %w", it.ID, it.Name, err)
		}
	}
	return nil
}

func sweepCredentials(_ string) error {
	c := testAccClient()
	if c == nil {
		return nil
	}
	ctx := context.Background()
	items, err := c.ListCredentials(ctx, client.CredentialListFilter{Search: sweepPrefix})
	if err != nil {
		return fmt.Errorf("sweep: listing credentials: %w", err)
	}
	for _, it := range items {
		if !strings.HasPrefix(it.Name, sweepPrefix) {
			continue
		}
		if err := c.DeleteCredential(ctx, it.ID); err != nil && !client.IsNotFound(err) {
			return fmt.Errorf("sweep: deleting credential %d (%s): %w", it.ID, it.Name, err)
		}
	}
	return nil
}

func sweepLocations(_ string) error {
	c := testAccClient()
	if c == nil {
		return nil
	}
	ctx := context.Background()
	items, err := c.ListLocations(ctx, client.LocationListFilter{Search: sweepPrefix})
	if err != nil {
		return fmt.Errorf("sweep: listing locations: %w", err)
	}
	// Delete children before parents: child locations carry a parent_id, so sweep
	// those first to avoid FK conflicts.
	var withParent, withoutParent []client.LocationResponse
	for _, it := range items {
		if !strings.HasPrefix(it.Name, sweepPrefix) {
			continue
		}
		if it.ParentID != nil {
			withParent = append(withParent, it)
		} else {
			withoutParent = append(withoutParent, it)
		}
	}
	for _, it := range append(withParent, withoutParent...) {
		if err := c.DeleteLocation(ctx, it.ID); err != nil && !client.IsNotFound(err) {
			return fmt.Errorf("sweep: deleting location %d (%s): %w", it.ID, it.Name, err)
		}
	}
	return nil
}

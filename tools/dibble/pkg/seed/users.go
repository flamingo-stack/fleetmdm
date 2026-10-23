package seed

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/fleetdm/fleet/v4/tools/dibble/pkg/themes"
)

// SeedUsers creates `count` users on the Fleet server using names drawn from
// the given theme. Roles cycle through observer / observer_plus / maintainer /
// admin / gitops so the seeded set covers every permission level.
//
// All users share a known dev password so tests can sign in as them; production
// Fleets should never run this against a real deployment. GitOps (api_only)
// users authenticate via API token only, so instead of the shared dev password
// they are seeded with a random, discarded password that nobody is expected
// to use to sign in interactively.
const SeededUserPassword = "DibbleSeed123!"

var seededRoles = []string{
	"observer", "observer_plus", "maintainer", "admin", "gitops",
}

// randomPassword generates a per-user random credential used only to satisfy
// Fleet's /users/admin endpoint requirement that a password be set on the
// record; it is never surfaced to the operator and cannot be used to log in
// since api_only accounts authenticate via API token.
func randomPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func Users(c Client, log Logger, theme themes.Theme, count int) Result {
	res := Result{Entity: "users"}
	for i := 0; i < count; i++ {
		name := themes.FullName(theme, i)
		email := themes.Email(theme, i)
		role := seededRoles[i%len(seededRoles)]
		body := map[string]any{
			"name":                        name,
			"email":                       email,
			"global_role":                 role,
			"admin_forced_password_reset": false,
			"password":                    SeededUserPassword,
		}
		// GitOps users authenticate via API token only, but Fleet's
		// /users/admin endpoint still requires a password be set on the
		// record (only SSO-enabled creates waive that requirement). Rather
		// than shipping the shared, publicly-known dev password to an
		// api_only account, generate a random, discarded credential per
		// user so it is not a usable shared secret.
		if role == "gitops" {
			body["api_only"] = true
			pw, err := randomPassword()
			if err != nil {
				res.Errors = append(res.Errors, err)
				continue
			}
			body["password"] = pw
		}
		err := c.Post("/api/latest/fleet/users/admin", body, nil)
		switch {
		case err == nil:
			res.Created++
			log.Printf("user %s <%s> [%s]", name, email, role)
		case IsAlreadyExists(err):
			res.Skipped++
		default:
			res.Errors = append(res.Errors, err)
		}
	}
	return res
}

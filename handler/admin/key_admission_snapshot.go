package admin

import "github.com/deliciousbuding/metapi-go/auth"

// keyAdmissionSnapshot returns process-local window usage for a downstream key.
func keyAdmissionSnapshot(keyID int64) (usedRPM, usedTPM int64) {
	return auth.GlobalKeyAdmission.Snapshot(keyID)
}

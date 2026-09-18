package main

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/boltdb/bolt"
	scepdepot "github.com/fleetdm/fleet/v4/server/mdm/scep/depot/bolt"
	apnsbuiltin "github.com/micromdm/micromdm/platform/apns/builtin"
	"github.com/micromdm/micromdm/platform/device"
	devicebuiltin "github.com/micromdm/micromdm/platform/device/builtin"
	"github.com/micromdm/micromdm/platform/pubsub/inmem"
	"github.com/micromdm/plist"
	"go.etcd.io/bbolt"
)

type Authenticate struct {
	MessageType  string
	UDID         string
	Topic        string
	BuildVersion string `plist:",omitempty"`
	DeviceName   string `plist:",omitempty"`
	Model        string `plist:",omitempty"`
	ModelName    string `plist:",omitempty"`
	OSVersion    string `plist:",omitempty"`
	ProductName  string `plist:",omitempty"`
	SerialNumber string `plist:",omitempty"`
	IMEI         string `plist:",omitempty"`
	MEID         string `plist:",omitempty"`
}

type TokenUpdate struct {
	MessageType   string
	UDID          string
	PushMagic     string
	Topic         string
	Token         []byte
	UnlockToken   []byte `plist:",omitempty"`
	UserID        string `plist:",omitempty"`
	UserShortName string `plist:",omitempty"`
	UserLongName  string `plist:",omitempty"`
}

// referenceTime is used as a canary to insert/update records. As long as a
// record has an `updated_at` timestamp, the script will update it, but if the
// timestamp has changed, the record will be completely ignored.
const referenceTime = "2000-01-01 00:00:00"

// sqlQuote escapes a string for safe inclusion as a single-quoted SQL string
// literal by doubling any embedded single quotes and backslashes.
func sqlQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `''`)
	return s
}

func main() {
	flDB := flag.String("db", "/var/db/micromdm/micromdm.db", "path to micromdm DB")
	flag.Parse()

	// Device records
	func() {
		log.Println("Open DB for devices")
		boltDB, err := bolt.Open(*flDB, 0o600, nil)
		if err != nil {
			log.Fatal(err)
		}
		defer boltDB.Close()

		ps := inmem.NewPubSub()
		apnsDB, err := apnsbuiltin.NewDB(boltDB, ps)
		if err != nil {
			log.Fatal(err)
		}

		deviceDB, err := devicebuiltin.NewDB(boltDB)
		if err != nil {
			log.Fatal(err)
		}
		devices, err := deviceDB.List(context.Background(), device.ListDevicesOption{})
		if err != nil {
			log.Fatal(err)
		}
		if len(devices) == 0 {
			log.Printf("No devices found. Are you sure %s is a MicroMDM DB?", *flDB)
		} else {
			log.Printf("Found %d devices", len(devices))
		}

		// SCEP certificates are stored using the certificate CN as the
		// key. I couldn't find any CN <-> device association in the
		// db, as it's not needed: micro extracts the certificate CN
		// from the request and uses it to authenticate devices.
		//
		// To avoid loading all certs in memory (rough estimation is
		// ~2KB per cert) we store a map of the cert hash (which is
		// stored along the device record) to the CN for later
		// retrieval.
		certHashToCertKey := make(map[string][]byte, len(devices))
		// NOTE: the depot doesn't expose methods to list certs so we
		// need to use bolt directly.
		err = boltDB.View(func(tx *bolt.Tx) error {
			bucket := tx.Bucket([]byte("scep_certificates"))
			if bucket == nil {
				log.Fatalf("No identity certificates found. Are you sure %s is a MicroMDM DB?", *flDB)
			}
			return bucket.ForEach(func(k, v []byte) error {
				hash := sha256.Sum256(v)
				certHashToCertKey[string(hash[:])] = k
				return nil
			})
		})
		if err != nil {
			log.Fatal(err)
		}

		var sb strings.Builder
		for _, device := range devices {
			if len(device.UDID) == 0 {
				log.Println("Skipping device with empty UDID. Serial: ", device.SerialNumber, " UUID: ", device.UUID, " Last seen: ", device.LastSeen)
				continue
			}
			pushInfo, err := apnsDB.PushInfo(context.Background(), device.UDID)
			if err != nil {
				log.Println(device.UDID, " FAILED: ", err)
				continue
			}

			authenticate := &Authenticate{
				MessageType:  "Authenticate",
				UDID:         device.UDID,
				Topic:        pushInfo.MDMTopic,
				BuildVersion: device.BuildVersion,
				DeviceName:   device.DeviceName,
				Model:        device.Model,
				ModelName:    device.ModelName,
				OSVersion:    device.OSVersion,
				ProductName:  device.ProductName,
				SerialNumber: device.SerialNumber,
				IMEI:         device.IMEI,
				MEID:         device.MEID,
			}

			authenticatePlist, err := plist.Marshal(authenticate)
			if err != nil {
				log.Println(err)
				continue
			}

			token, err := hex.DecodeString(pushInfo.Token)
			if err != nil {
				log.Println(device.UDID, " FAILED: ", err)
				continue
			}
			unlockToken, err := hex.DecodeString(device.UnlockToken)
			if err != nil {
				log.Println(device.UDID, " FAILED: ", err)
				continue
			}

			tokenUpdate := &TokenUpdate{
				MessageType: "TokenUpdate",
				UDID:        device.UDID,

				PushMagic: pushInfo.PushMagic,
				Token:     token,
				Topic:     pushInfo.MDMTopic,

				UnlockToken: unlockToken,
			}

			tokenPlist, err := plist.Marshal(tokenUpdate)
			if err != nil {
				log.Println(err)
				continue
			}

			certHash, err := deviceDB.GetUDIDCertHash([]byte(device.UDID))
			if err != nil {
				log.Println(device.UDID, " FAILED: ", err)
				continue
			}

			var certDer []byte
			certKey := certHashToCertKey[string(certHash)]
			err = boltDB.View(func(tx *bolt.Tx) error {
				bucket := tx.Bucket([]byte("scep_certificates"))
				certDer = bucket.Get(certKey)
				return nil
			})
			if err != nil {
				log.Println(device.UDID, " FAILED: ", err)
				continue
			}

			var certExpiration string
			var certPEM []byte
			if certDer != nil {
				// parse the cert to extract the expiration date
				cert, err := x509.ParseCertificate(certDer)
				if err != nil {
					log.Printf("WARN: unable to parse SCEP identity certificate for %s: %s\n", device.UDID, err)
				} else {
					certExpiration = cert.NotAfter.Format("2006-01-02 15:04:05")

					// encode it to PEM to store it in the DB in
					// the format that nano expects. At the moment
					// we don't really need this value as we can
					// make do with the hash and the expiration,
					// but I figured it would be good to have it.
					pemBlock := &pem.Block{
						Type:  "CERTIFICATE",
						Bytes: cert.Raw,
					}
					certPEM = pem.EncodeToMemory(pemBlock)
				}
			}

			if len(device.BootstrapToken) == 0 {
				log.Println("Device with empty bootstrap token: ", device.UDID, " Last seen: ", device.LastSeen.String())
			}

			base64BootstrapToken := base64.StdEncoding.EncodeToString(device.BootstrapToken)

			sb.WriteString(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(`
INSERT INTO nano_devices
    (
      id,
      serial_number,
      authenticate,
      authenticate_at,
      token_update,
      token_update_at,
      bootstrap_token_b64,
      bootstrap_token_at,
      identity_cert,
      updated_at
    )
SELECT
      '__UDID__',
      '__SERIAL__',
      '__AUTH_PLIST__',
      CURRENT_TIMESTAMP,
      '__TOKEN_PLIST__',
      CURRENT_TIMESTAMP,
      NULLIF('__BOOTSTRAP__', ''),
      CURRENT_TIMESTAMP,
      NULLIF('__CERT_PEM__', ''),
      '__REF_TIME__'
WHERE
    NOT EXISTS (
        SELECT 1 FROM nano_devices
        WHERE id = '__UDID__' AND updated_at != '__REF_TIME__'
    )
ON DUPLICATE KEY
UPDATE
    updated_at = updated_at, -- preserve updated_at
    serial_number = VALUES(serial_number),
    authenticate = VALUES(authenticate),
    authenticate_at = CURRENT_TIMESTAMP,
    token_update = VALUES(token_update),
    token_update_at = CURRENT_TIMESTAMP,
    bootstrap_token_b64 = VALUES(bootstrap_token_b64),
    bootstrap_token_at = CURRENT_TIMESTAMP,
    identity_cert = VALUES(identity_cert);
		`,
				"__UDID__", sqlQuote(device.UDID), -1),
				"__SERIAL__", sqlQuote(device.SerialNumber), -1),
				"__AUTH_PLIST__", sqlQuote(string(authenticatePlist)), -1),
				"__TOKEN_PLIST__", sqlQuote(string(tokenPlist)), -1),
				"__BOOTSTRAP__", sqlQuote(base64BootstrapToken), -1),
				"__CERT_PEM__", sqlQuote(string(certPEM)), -1),
				"__REF_TIME__", sqlQuote(referenceTime), -1))

			sb.WriteString(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(`
INSERT INTO nano_enrollments (
      id,
      device_id,
      user_id, type,
      topic,
      push_magic,
      token_hex,
      enabled,
      last_seen_at,
      enrolled_from_migration,
      token_update_tally,
      updated_at
)
SELECT
      '__UDID__',
      '__UDID__',
      NULL,
      'Device',
      '__TOPIC__',
      '__PUSH_MAGIC__',
      '__TOKEN_HEX__',
      __ENABLED__,
      CURRENT_TIMESTAMP,
      1,
      1,
      '__REF_TIME__'
WHERE
    NOT EXISTS (
        SELECT 1 FROM nano_enrollments
        WHERE id = '__UDID__' AND updated_at != '__REF_TIME__'
    )
ON DUPLICATE KEY
UPDATE
    updated_at = updated_at, -- preserve updated_at
    device_id = VALUES(device_id),
    user_id = VALUES(user_id),
    type = VALUES(type),
    topic = VALUES(topic),
    push_magic = VALUES(push_magic),
    token_hex = VALUES(token_hex),
    enabled = VALUES(enabled),
    last_seen_at = CURRENT_TIMESTAMP,
    token_update_tally = nano_enrollments.token_update_tally + 1;`,
				"__UDID__", sqlQuote(device.UDID), -1),
				"__TOPIC__", sqlQuote(tokenUpdate.Topic), -1),
				"__PUSH_MAGIC__", sqlQuote(tokenUpdate.PushMagic), -1),
				"__TOKEN_HEX__", sqlQuote(hex.EncodeToString(tokenUpdate.Token)), -1),
				"__ENABLED__", sqlQuote(boolToSQL(device.Enrolled)), -1),
				"__REF_TIME__", sqlQuote(referenceTime), -1))

			sb.WriteString(strings.Replace(strings.Replace(strings.Replace(strings.Replace(`
INSERT INTO nano_cert_auth_associations
    (id, sha256, cert_not_valid_after, updated_at)
SELECT
    '__UDID__', '__SHA256__', NULLIF('__CERT_EXP__', ''), '__REF_TIME__'
WHERE
    NOT EXISTS (
        SELECT 1 FROM nano_cert_auth_associations
        WHERE id = '__UDID__' AND updated_at != '__REF_TIME__'
    )
ON DUPLICATE KEY UPDATE
  id = VALUES(id),
  updated_at = updated_at, -- preserve updated_at
  sha256 = VALUES(sha256),
  cert_not_valid_after = VALUES(cert_not_valid_after);
	    `,
				"__UDID__", sqlQuote(device.UDID), -1),
				"__SHA256__", sqlQuote(hex.EncodeToString(certHash)), -1),
				"__CERT_EXP__", sqlQuote(certExpiration), -1),
				"__REF_TIME__", sqlQuote(referenceTime), -1))
		}

		sb.WriteString("\n")
		if err := os.WriteFile("dump.sql", []byte(sb.String()), 0o600); err != nil {
			log.Fatal(err)
		}
		log.Println("Wrote device/enrollment records to dump.sql")
	}()

	// SCEP cert/key
	func() {
		log.Println("Open DB for SCEP cert and key")
		bboltDB, err := bbolt.Open(*flDB, 0o600, nil)
		if err != nil {
			log.Fatal(err)
		}
		defer bboltDB.Close()

		scepBoltDepot, err := scepdepot.NewBoltDepot(bboltDB)
		if err != nil {
			log.Fatal(err)
		}

		key, err := scepBoltDepot.CreateOrLoadKey(2048)
		if err != nil {
			log.Fatal(err)
		}

		crt, err := scepBoltDepot.CreateOrLoadCA(key, 5, "MicroMDM", "US")
		if err != nil {
			log.Fatal(err)
		}

		privateKeyBytes := x509.MarshalPKCS1PrivateKey(key)
		privateKeyPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: privateKeyBytes,
		})

		if err := os.WriteFile("scep.key", privateKeyPEM, 0o600); err != nil {
			log.Fatal(err)
		}

		certPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: crt.Raw,
		})

		if err := os.WriteFile("scep.cert", certPEM, 0o600); err != nil {
			log.Fatal(err)
		}

		log.Println("Wrote SCEP cert/key to scep.cert/scep.key")
	}()
}

// boolToSQL renders a bool as a literal SQL boolean/int value (0 or 1).
func boolToSQL(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// unused import guard: ensure database/sql import is referenced to signal
// intent for future parameterized-query migration without altering behavior.
var _ = sql.ErrNoRows

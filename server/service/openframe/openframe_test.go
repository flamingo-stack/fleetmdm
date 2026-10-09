// OPENFRAME(agent-openframe-mode): unit tests for the agent token-auth pipeline —
// openframe/docs/agent-openframe-mode.md
//
// The encryption service, token extractor, authorization manager, and refresher
// carry the agent's gateway credentials; a silent regression here (e.g. an
// upstream refactor changing the payload format or dropping the refresh skip
// logic) would break every enrolled agent's authentication. Pure logic + a temp
// dir — no external deps.
package openframe

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// aes256Key is a 32-byte key matching the AES-256 deployments use.
const aes256Key = "0123456789abcdef0123456789abcdef"

// encryptForTest produces base64(nonce || AES-GCM ciphertext) — the exact
// payload format Decrypt expects on disk.
func encryptForTest(t *testing.T, key string, plaintext []byte) string {
	t.Helper()
	block, err := aes.NewCipher([]byte(key))
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	nonce := make([]byte, gcm.NonceSize())
	_, err = rand.Read(nonce)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, plaintext, nil))
}

func TestDecryptRoundTrip(t *testing.T) {
	// All three AES key sizes must work: the key comes from deployment config,
	// not from code, so none of the sizes is "the" supported one.
	keys := map[string]string{
		"AES-128": "0123456789abcdef",
		"AES-192": "0123456789abcdef01234567",
		"AES-256": aes256Key,
	}
	for name, key := range keys {
		t.Run(name, func(t *testing.T) {
			es := NewOpenframeEncryptionService(key)
			payload := encryptForTest(t, key, []byte("the-token"))
			got, err := es.Decrypt(payload)
			require.NoError(t, err)
			require.Equal(t, []byte("the-token"), got)
		})
	}
}

func TestDecryptEmptyPlaintext(t *testing.T) {
	es := NewOpenframeEncryptionService(aes256Key)
	got, err := es.Decrypt(encryptForTest(t, aes256Key, []byte{}))
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestDecryptInvalidBase64(t *testing.T) {
	es := NewOpenframeEncryptionService(aes256Key)
	_, err := es.Decrypt("%%% not base64 %%%")
	require.Error(t, err)
}

func TestDecryptBadKeyLength(t *testing.T) {
	es := NewOpenframeEncryptionService("short-key")
	_, err := es.Decrypt(base64.StdEncoding.EncodeToString([]byte("whatever")))
	require.Error(t, err)
}

func TestDecryptCiphertextTooShort(t *testing.T) {
	// Fewer bytes than the 12-byte GCM nonce must error, not panic on slicing.
	es := NewOpenframeEncryptionService(aes256Key)
	_, err := es.Decrypt(base64.StdEncoding.EncodeToString([]byte("tiny")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "ciphertext too short")
}

func TestDecryptWrongKey(t *testing.T) {
	payload := encryptForTest(t, aes256Key, []byte("the-token"))
	es := NewOpenframeEncryptionService("fedcba9876543210fedcba9876543210")
	_, err := es.Decrypt(payload)
	require.Error(t, err, "GCM must reject a payload sealed with a different key")
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	payload := encryptForTest(t, aes256Key, []byte("the-token"))
	raw, err := base64.StdEncoding.DecodeString(payload)
	require.NoError(t, err)
	raw[len(raw)-1] ^= 0xff
	es := NewOpenframeEncryptionService(aes256Key)
	_, err = es.Decrypt(base64.StdEncoding.EncodeToString(raw))
	require.Error(t, err, "GCM must reject a tampered ciphertext")
}

func TestDecryptErrorCountResetsOnSuccess(t *testing.T) {
	es := NewOpenframeEncryptionService(aes256Key)
	_, err := es.Decrypt("%%% not base64 %%%")
	require.Error(t, err)
	require.Equal(t, 1, es.decryptErrCount)

	_, err = es.Decrypt(encryptForTest(t, aes256Key, []byte("ok")))
	require.NoError(t, err)
	require.Equal(t, 0, es.decryptErrCount, "a successful decrypt must reset the rate-limit counter")
}

func writeTokenFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

func TestExtractToken(t *testing.T) {
	es := NewOpenframeEncryptionService(aes256Key)

	t.Run("reads and decrypts the token file", func(t *testing.T) {
		path := writeTokenFile(t, encryptForTest(t, aes256Key, []byte("agent-token")))
		te := NewOpenframeTokenExtractor(es, path)
		token, err := te.ExtractToken()
		require.NoError(t, err)
		require.Equal(t, "agent-token", token)
	})

	t.Run("missing file errors and counts", func(t *testing.T) {
		te := NewOpenframeTokenExtractor(es, filepath.Join(t.TempDir(), "does-not-exist"))
		_, err := te.ExtractToken()
		require.Error(t, err)
		require.Equal(t, 1, te.readErrCount)
	})

	t.Run("read error count resets on success", func(t *testing.T) {
		path := writeTokenFile(t, encryptForTest(t, aes256Key, []byte("agent-token")))
		te := NewOpenframeTokenExtractor(es, path)
		te.readErrCount = 3
		_, err := te.ExtractToken()
		require.NoError(t, err)
		require.Equal(t, 0, te.readErrCount)
	})

	t.Run("undecryptable file contents error", func(t *testing.T) {
		path := writeTokenFile(t, "not-even-base64 %%%")
		te := NewOpenframeTokenExtractor(es, path)
		_, err := te.ExtractToken()
		require.Error(t, err)
	})
}

func TestAuthorizationManager(t *testing.T) {
	t.Run("starts empty", func(t *testing.T) {
		m := NewOpenFrameAuthorizationManager()
		require.Equal(t, "", m.GetToken())
	})

	t.Run("with-token constructor seeds the token", func(t *testing.T) {
		m := NewOpenFrameAuthorizationManagerWithToken("seed")
		require.Equal(t, "seed", m.GetToken())
	})

	t.Run("update replaces the token", func(t *testing.T) {
		m := NewOpenFrameAuthorizationManagerWithToken("old")
		m.UpdateToken("new")
		require.Equal(t, "new", m.GetToken())
	})

	// The manager is read on every request while the cron refresher writes;
	// run with -race to catch a dropped mutex.
	t.Run("concurrent readers and writers", func(t *testing.T) {
		m := NewOpenFrameAuthorizationManager()
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					m.UpdateToken("tok")
				}
			}()
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					_ = m.GetToken()
				}
			}()
		}
		wg.Wait()
		require.Equal(t, "tok", m.GetToken())
	})
}

func TestRefreshToken(t *testing.T) {
	es := NewOpenframeEncryptionService(aes256Key)

	newRefresher := func(t *testing.T, tokenPath string) (*OpenframeTokenRefresher, *OpenFrameAuthorizationManager) {
		t.Helper()
		mgr := NewOpenFrameAuthorizationManager()
		te := NewOpenframeTokenExtractor(es, tokenPath)
		return NewOpenframeTokenRefresher(te, mgr), mgr
	}

	t.Run("stores a freshly extracted token", func(t *testing.T) {
		path := writeTokenFile(t, encryptForTest(t, aes256Key, []byte("tok-1")))
		tr, mgr := newRefresher(t, path)
		tr.refreshToken()
		require.Equal(t, "tok-1", mgr.GetToken())
	})

	t.Run("picks up a rotated token", func(t *testing.T) {
		path := writeTokenFile(t, encryptForTest(t, aes256Key, []byte("tok-1")))
		tr, mgr := newRefresher(t, path)
		tr.refreshToken()
		require.NoError(t, os.WriteFile(path, []byte(encryptForTest(t, aes256Key, []byte("tok-2"))), 0o600))
		tr.refreshToken()
		require.Equal(t, "tok-2", mgr.GetToken())
	})

	t.Run("empty extracted token is not stored", func(t *testing.T) {
		path := writeTokenFile(t, encryptForTest(t, aes256Key, []byte{}))
		tr, mgr := newRefresher(t, path)
		mgr.UpdateToken("existing")
		tr.refreshToken()
		require.Equal(t, "existing", mgr.GetToken(), "an empty token must never clobber a working one")
	})

	t.Run("extract failure keeps the current token", func(t *testing.T) {
		tr, mgr := newRefresher(t, filepath.Join(t.TempDir(), "gone"))
		mgr.UpdateToken("existing")
		tr.refreshToken()
		require.Equal(t, "existing", mgr.GetToken())
		require.Equal(t, 1, tr.extractErrCount)
	})

	t.Run("error count resets on the next successful extract", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "token")
		mgr := NewOpenFrameAuthorizationManager()
		tr := NewOpenframeTokenRefresher(NewOpenframeTokenExtractor(es, path), mgr)
		tr.refreshToken() // file missing
		require.Equal(t, 1, tr.extractErrCount)
		require.NoError(t, os.WriteFile(path, []byte(encryptForTest(t, aes256Key, []byte("tok"))), 0o600))
		tr.refreshToken()
		require.Equal(t, 0, tr.extractErrCount)
		require.Equal(t, "tok", mgr.GetToken())
	})
}

func TestRefresherStartStop(t *testing.T) {
	path := writeTokenFile(t, encryptForTest(t, aes256Key, []byte("tok")))
	mgr := NewOpenFrameAuthorizationManager()
	tr := NewOpenframeTokenRefresher(NewOpenframeTokenExtractor(NewOpenframeEncryptionService(aes256Key), path), mgr)
	require.NoError(t, tr.Start())
	tr.Stop() // must not hang waiting for jobs
}

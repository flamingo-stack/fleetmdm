//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
)

import (
	"github.com/fleetdm/fleet/v4/orbit/pkg/dialog"
	"github.com/fleetdm/fleet/v4/orbit/pkg/lvm"
	"github.com/fleetdm/fleet/v4/orbit/pkg/zenity"
	"github.com/siderolabs/go-blockdevice/v2/encryption"
	"github.com/siderolabs/go-blockdevice/v2/encryption/luks"
)

const maxKeySlots = 8
const maxPassphraseRetries = 3

func run() error {
	devicePath, err := lvm.FindRootDisk()
	if err != nil {
		return fmt.Errorf("find root disk: %w", err)
	}

	prompt := zenity.New()

	// Prompt existing passphrase from the user.
	currentPassphrase, err := prompt.ShowEntry(dialog.EntryOptions{
		Title:    "Enter Existing LUKS Passphrase",
		Text:     "Enter your existing LUKS passphrase:",
		HideText: true,
	})
	if err != nil {
		return fmt.Errorf("show entry dialog: %w", err)
	}

	escrowPassPhrase, err := generateEscrowPassphrase()
	if err != nil {
		return fmt.Errorf("generate escrow passphrase: %w", err)
	}

	// TODO: submit escrowPassPhrase to the secure secret store / server
	// rather than only holding it in memory here.

	device := luks.New(luks.AESXTSPlain64Cipher)

	keySlot := 1
	passphraseRetries := 0
	for {
		if keySlot >= maxKeySlots {
			return errors.New("all LUKS key slots are full")
		}

		userKey := encryption.NewKey(0, currentPassphrase)
		escrowKey := encryption.NewKey(keySlot, []byte(escrowPassPhrase))

		if err := device.AddKey(context.Background(), devicePath, userKey, escrowKey); err != nil {
			if errors.Is(err, encryption.ErrEncryptionKeyRejected) {
				passphraseRetries++
				if passphraseRetries > maxPassphraseRetries {
					return fmt.Errorf("add key: too many incorrect passphrase attempts: %w", err)
				}

				currentPassphrase, err = prompt.ShowEntry(dialog.EntryOptions{
					Title:    "Enter Existing LUKS Passphrase",
					Text:     "Bad password. Enter your existing LUKS passphrase:",
					HideText: true,
				})
				if err != nil {
					return fmt.Errorf("show retry entry dialog: %w", err)
				}
				continue
			}

			fmt.Println("add key err:", err)
			keySlot++
			continue
		}

		break
	}

	fmt.Println("Key escrowed successfully.")
	return nil
}

func generateEscrowPassphrase() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func main() {
	if err := run(); err != nil {
		fmt.Println("luks escrow error:", err)
		os.Exit(1)
	}
}

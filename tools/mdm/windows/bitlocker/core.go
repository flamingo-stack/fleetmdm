package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"math/big"
	"strings"
)

func BitlockerEncryptionNumericalPassword(encryptionPassword string) error {

	// Connect to the volume
	vol, err := Connect("c:")
	if err != nil {
		return fmt.Errorf("there was an error connecting to the volume - error: %w", err)
	}
	defer vol.Close()

	// Prepare for encryption
	if err := vol.Prepare(VolumeTypeDefault, EncryptionTypeSoftware); err != nil {
		return fmt.Errorf("there was an error preparing the volume for encryption - error: %w", err)
	}

	// Add a recovery protector

	if err := vol.ProtectWithNumericalPassword(encryptionPassword); err != nil {
		return fmt.Errorf("there was an error adding a recovery protector - error: %w", err)
	}

	// Protect with TPM
	if err := vol.ProtectWithTPM(nil); err != nil {
		return fmt.Errorf("there was an error protecting with TPM - error: %w", err)
	}

	// Start encryption
	if err := vol.Encrypt(XtsAES256, EncryptDataOnly); err != nil {
		return fmt.Errorf("there was an error starting encryption - error: %w", err)
	}

	return nil
}

func BitlockerDecryption() error {

	// Connect to the volume
	vol, err := Connect("c:")
	if err != nil {
		return fmt.Errorf("there was an error connecting to the volume - error: %w", err)
	}
	defer vol.Close()

	// Start decryption
	if err := vol.Decrypt(); err != nil {
		return fmt.Errorf("there was an error starting decryption - error: %w", err)
	}

	return nil
}

func GetBitlockerStatus() (*EncryptionStatus, error) {

	// Connect to the volume
	vol, err := Connect("c:")
	if err != nil {
		return nil, fmt.Errorf("there was an error connecting to the volume - error: %w", err)
	}
	defer vol.Close()

	// Get volume status
	status, err := vol.GetBitlockerStatus()
	if err != nil {
		return nil, fmt.Errorf("there was an error starting decryption - error: %w", err)
	}

	return status, nil
}

// generateNumericalRecoveryPassword generates a BitLocker-style 48-digit
// numerical recovery password, formatted as 8 groups of 6 digits separated
// by dashes, following the algorithm referenced at
// https://learn.microsoft.com/en-us/windows/win32/secprov/getkeyprotectornumericalpassword-win32-encryptablevolume
func generateNumericalRecoveryPassword() (string, error) {
	groups := make([]string, 8)
	for i := 0; i < 8; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(1000000))
		if err != nil {
			return "", fmt.Errorf("generating random recovery password group - error: %w", err)
		}
		groups[i] = fmt.Sprintf("%06d", n.Int64())
	}
	return strings.Join(groups, "-"), nil
}

func main() {

	enableBitlocker := flag.Bool("encrypt", false, "encrypt the drive")
	disableBitlocker := flag.Bool("decrypt", false, "decrypt the drive")
	statusBitlocker := flag.Bool("status", true, "get drive status")

	flag.Parse()

	if *enableBitlocker {
		fmt.Println("About to attempt enabling bitlocker")

		//This needs to be generated with algorithm defined at
		//https://learn.microsoft.com/en-us/windows/win32/secprov/getkeyprotectornumericalpassword-win32-encryptablevolume
		newPassword, err := generateNumericalRecoveryPassword()
		if err != nil {
			fmt.Printf("bitlocker recovery password generation error - %v\n", err)
			return
		}

		err = BitlockerEncryptionNumericalPassword(newPassword)
		if err != nil {
			fmt.Printf("bitlocker encryption error - %v\n", err)
			return
		}

		fmt.Println("Bitlocker encryption started!")

	} else if *disableBitlocker {
		fmt.Println("About to attempt disabling bitlocker")

		err := BitlockerDecryption()
		if err != nil {
			fmt.Printf("bitlocker decryption error - %v\n", err)
			return
		}

		fmt.Println("Bitlocker decryption started!")

	} else if *statusBitlocker {
		fmt.Println("About to get encryption status bitlocker")

		status, err := GetBitlockerStatus()
		if err != nil {
			fmt.Printf("bitlocker decryption error - %v\n", err)
			return
		}

		fmt.Println("Protection status: ", status.ProtectionStatusDesc)
		fmt.Println("Conversion status: ", status.ConversionStatusDesc)
		fmt.Println("Encryption Flags: ", status.EncryptionFlags)
		fmt.Println("Wiping Status description: ", status.WipingStatusDesc)
		fmt.Println("Encryption percentage complete: ", status.EncryptionPercentage)
		fmt.Println("Wiping percentage complete: ", status.WipingPercentage)

		fmt.Println("Bitlocker encryption status gathered!")

	} else {
		fmt.Println("You must specify either -encrypt, -decrypt or -status")
		return
	}
}

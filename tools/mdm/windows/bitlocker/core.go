package main

import (
	"crypto/rand"
	"flag"
	"fmt"
)

func BitlockerEncryptionNumericalPassword(encryptionPassword string) error {

	// Connect to the volume
	vol, err := Connect("c:")
	if err != nil {
		return fmt.Errorf("there was an error connecting to the volume: %w", err)
	}
	defer vol.Close()

	// Prepare for encryption
	if err := vol.Prepare(VolumeTypeDefault, EncryptionTypeSoftware); err != nil {
		return fmt.Errorf("there was an error preparing the volume for encryption: %w", err)
	}

	// Add a recovery protector

	if err := vol.ProtectWithNumericalPassword(encryptionPassword); err != nil {
		return fmt.Errorf("there was an error adding a recovery protector: %w", err)
	}

	// Protect with TPM
	if err := vol.ProtectWithTPM(nil); err != nil {
		return fmt.Errorf("there was an error protecting with TPM: %w", err)
	}

	// Start encryption
	if err := vol.Encrypt(XtsAES256, EncryptDataOnly); err != nil {
		return fmt.Errorf("there was an error starting encryption: %w", err)
	}

	return nil
}

func BitlockerDecryption() error {

	// Connect to the volume
	vol, err := Connect("c:")
	if err != nil {
		return fmt.Errorf("there was an error connecting to the volume: %w", err)
	}
	defer vol.Close()

	// Start decryption
	if err := vol.Decrypt(); err != nil {
		return fmt.Errorf("there was an error starting decryption: %w", err)
	}

	return nil
}

func GetBitlockerStatus() (*EncryptionStatus, error) {

	// Connect to the volume
	vol, err := Connect("c:")
	if err != nil {
		return nil, fmt.Errorf("there was an error connecting to the volume: %w", err)
	}
	defer vol.Close()

	// Get volume status
	status, err := vol.GetBitlockerStatus()
	if err != nil {
		return nil, fmt.Errorf("there was an error getting bitlocker status: %w", err)
	}

	return status, nil
}

// generateNumericalRecoveryPassword generates a random 48-digit BitLocker
// numerical recovery password formatted as 8 groups of 6 digits, per
// https://learn.microsoft.com/en-us/windows/win32/secprov/getkeyprotectornumericalpassword-win32-encryptablevolume
func generateNumericalRecoveryPassword() (string, error) {
	const groups = 8
	password := ""
	for i := 0; i < groups; i++ {
		if i > 0 {
			password += "-"
		}

		max := int64(1000000) // 6 digits, 0-999999
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("there was an error generating a random recovery password: %w", err)
		}

		var n int64
		for _, v := range b {
			n = (n << 8) | int64(v)
		}
		if n < 0 {
			n = -n
		}
		n = n % max

		password += fmt.Sprintf("%06d", n)
	}

	return password, nil
}

func main() {

	enableBitlocker := flag.Bool("encrypt", false, "encrypt the drive")
	disableBitlocker := flag.Bool("decrypt", false, "decrypt the drive")
	statusBitlocker := flag.Bool("status", true, "get drive status")

	flag.Parse()

	if *enableBitlocker {
		fmt.Println("About to attempt enabling bitlocker")

		newPassword, err := generateNumericalRecoveryPassword()
		if err != nil {
			fmt.Printf("bitlocker encryption error - %v\n", err)
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

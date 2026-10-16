package file

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/fleetdm/fleet/v4/server/mdm/nanomdm/mdm"
)

func (s *FileStorage) EnrollmentHasCertHash(r *mdm.Request, _ string) (bool, error) {
	e := s.newEnrollment(r.ID)
	_, err := e.readFile(CertAuthFilename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, err
}

func (s *FileStorage) HasCertHash(r *mdm.Request, hash string) (bool, error) {
	f, err := os.Open(path.Join(s.path, CertAuthAssociationsFilename))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ",", 2)
		if len(parts) == 2 && parts[1] == hash {
			return true, nil
		}
	}
	return false, scanner.Err()
}

func (s *FileStorage) IsCertHashAssociated(r *mdm.Request, hash string) (bool, error) {
	e := s.newEnrollment(r.ID)
	b, err := e.readFile(CertAuthFilename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return strings.EqualFold(string(b), hash), nil
}

func (s *FileStorage) AssociateCertHash(r *mdm.Request, hash string, _ time.Time) error {
	f, err := os.OpenFile( // nolint:gosec // G302
		path.Join(s.path, CertAuthAssociationsFilename),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		return fmt.Errorf("opening cert auth associations file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(r.ID + "," + hash + "\n"); err != nil {
		return fmt.Errorf("writing cert auth association: %w", err)
	}
	e := s.newEnrollment(r.ID)
	if err := e.writeFile(CertAuthFilename, []byte(hash)); err != nil {
		return fmt.Errorf("writing cert auth file: %w", err)
	}
	return nil
}

func (s *FileStorage) EnrollmentFromHash(_ context.Context, hash string) (string, error) {
	f, err := os.Open(path.Join(s.path, CertAuthAssociationsFilename))
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		split := strings.SplitN(text, ",", 2)
		if len(split) < 2 {
			continue
		}
		if split[1] == hash {
			return split[0], nil
		}
	}
	return "", nil
}

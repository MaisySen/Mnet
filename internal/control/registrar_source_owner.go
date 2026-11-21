package control

import (
	"context"
	"crypto/sha256"
	"io"
	"os"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	source "fintex.au/mnet/internal/enrollmentsource"
	"fintex.au/mnet/internal/privatefs"
)

// Private Store-owned HTTP lifecycle, not installed-policy authority, evidence
// permit or enrollment writer. No production caller or startup read exists.
type registrarSourceOwner struct {
	pool     *source.Pool
	revision [32]byte
	retired  bool
}

func (s *Store) registrarSourcesLocked() (*registrarSourceOwner, error) {
	if err := s.healthLocked(); err != nil {
		return nil, err
	}
	previous := s.registrarSources
	if previous != nil && previous.retired {
		return nil, errRegistrarSource
	}
	raw, err := readRegistrarSources(s.ownedPath + ".registrar-sources-v1.json")
	if err != nil {
		if previous != nil {
			previous.retired = true
			previous.pool.Close()
		}
		if os.IsNotExist(err) && previous == nil {
			return nil, errRegistrarSourceOff
		}
		return nil, errRegistrarSource
	}
	revision := sha256.Sum256(raw)
	if previous != nil {
		if revision != previous.revision {
			previous.retired = true
			previous.pool.Close()
			return nil, errRegistrarSource
		}
		return previous, nil
	}
	entries, limits, err := parseRegistrarSources(raw)
	if err != nil {
		return nil, err
	}
	pool, err := source.NewPool(entries, limits)
	if err != nil {
		return nil, errRegistrarSource
	}
	if err := s.healthLocked(); err != nil {
		pool.Close()
		return nil, err
	}
	owner := &registrarSourceOwner{pool: pool, revision: revision}
	s.registrarSources = owner
	return owner, nil
}

// reserveRegistrarSource is PRIVATE unused composition. Source alias resolves
// only configured scope. No caller can pass a client, policy or clock override.
// Result is observed DATA, never an inaccessible verified-evidence token.
func (s *Store) reserveRegistrarSource(ctx context.Context, sourceID string, request contract.SourceReserveRequest) (contract.SourceReserve, error) {
	s.mu.Lock()
	owner, err := s.registrarSourcesLocked()
	s.mu.Unlock()
	if err != nil {
		return contract.SourceReserve{}, err
	}
	data, err := owner.pool.Reserve(ctx, sourceID, request, time.Now())
	s.mu.Lock()
	defer s.mu.Unlock()
	current, currentErr := s.registrarSourcesLocked()
	if currentErr != nil || current != owner || err != nil {
		return contract.SourceReserve{}, source.ErrUnavailable
	}
	return data, nil
}
func readRegistrarSources(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > registrarSourceMaxBytes || singleLink(path, before) != nil {
		return nil, errRegistrarSource
	}
	if err := privatefs.ProtectFile(path); err != nil {
		return nil, errRegistrarSource
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, errRegistrarSource
	}
	raw, err := io.ReadAll(io.LimitReader(f, registrarSourceMaxBytes+1))
	if err != nil || len(raw) > registrarSourceMaxBytes {
		return nil, errRegistrarSource
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, after) || after.Size() != int64(len(raw)) || singleLink(path, after) != nil {
		return nil, errRegistrarSource
	}
	if err := f.Close(); err != nil {
		return nil, errRegistrarSource
	}
	return raw, nil
}

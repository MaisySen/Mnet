package enrollmentpending

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	registrar "fintex.au/mnet/internal/enrollmentregistrar"
)

func workflowJournalFaults(t *testing.T, s *httptest.Server, active *atomic.Value) {
	for _, when := range []string{"begin-marker", "challenge-pin", "submit-marker", "receipt-pin"} {
		for _, stage := range []string{"create", "write", "short", "sync", "close", "rename", "after-rename", "dirsync"} {
			t.Run(when+"-"+stage, func(t *testing.T) {
				j, _, cfg := bootstrapReady(t, s.URL)
				dir := j.owner.Parent()
				original, _ := os.ReadFile(j.path)
				var sends int
				var c contract.Challenge
				inject := func() {
					real := j.fs
					j.fs.create = func(dir string) (tempFile, error) {
						if stage == "create" {
							return nil, injected
						}
						f, err := real.create(dir)
						if err != nil {
							return nil, err
						}
						return &wrappedTemp{f, stage}, nil
					}
					if stage == "rename" {
						j.fs.rename = func(string, string) error { return injected }
					}
					if stage == "after-rename" {
						j.fs.rename = func(a, b string) error {
							if err := real.rename(a, b); err != nil {
								return err
							}
							return injected
						}
					}
					if stage == "dirsync" {
						j.fs.syncDir = func(string) error { return injected }
					}
				}
				active.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					sends++
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Cache-Control", "no-store")
					if r.URL.Path == registrar.WorkflowPrefix+"begin" {
						var input contract.WorkflowBeginWire
						json.NewDecoder(r.Body).Decode(&input)
						raw, _ := os.ReadFile(j.path)
						var saved record
						if !strictRecord(raw, &saved) || saved.Phase != "workflow-begin-may-have-started" {
							t.Error("no durable begin marker")
						}
						var own contract.SourceExchange
						json.Unmarshal([]byte(saved.ExchangeJSON), &own)
						descriptor, _ := contract.SourceDescriptorDigest(own, saved.Policy.ServiceID, input.Request.ReserveID)
						p, m := own.Provenance, own.Mapping
						now := time.Now().UnixMilli()
						c = contract.Challenge{ID: code(), Nonce: code(), EphemeralPublicKey: code(), ExpiresAtMS: now + 5000, Binding: contract.Binding{PrincipalAuthority: p.PrincipalAuthority, PrincipalNamespace: p.PrincipalNamespace, PrincipalID: p.PrincipalID, SessionBinding: p.SessionBinding, IssuerLabel: m.Issuer, RegistrarOrigin: m.RegistrarOrigin, Audience: m.Audience, ServiceID: saved.Policy.ServiceID, WorkspaceID: m.WorkspaceID, NetworkID: m.NetworkID, MappingRevision: m.MappingRevision, HandoffID: own.ID, AttemptID: own.Launch.AttemptID, ReserveID: input.Request.ReserveID, DescriptorDigest: descriptor, DescriptorEncodingVersion: 1, DevicePublicKey: own.Launch.DevicePublicKey, CredentialDigest: own.Launch.CredentialDigest, CheckedAtMS: now, ReserveDeadlineMS: now + 30000, AccessUntilMS: now + 5000}}
						if when == "challenge-pin" {
							j.mu.Lock()
							inject()
							j.mu.Unlock()
						}
						json.NewEncoder(w).Encode(contract.ChallengeWire{Version: 4, Challenge: c})
						return
					}
					raw, _ := os.ReadFile(j.path)
					var saved record
					if !strictRecord(raw, &saved) || saved.Phase != "workflow-submit-may-have-started" {
						t.Error("no durable submit marker")
					}
					if when == "receipt-pin" {
						j.mu.Lock()
						inject()
						j.mu.Unlock()
					}
					receipt, err := contract.ReceiptV2ForChallenge(c, "node-fixture", code(), time.Now().UnixMilli())
					if err != nil {
						t.Error(err)
					}
					json.NewEncoder(w).Encode(contract.WorkflowReceiptWire{Version: 1, Receipt: receipt, ReceiptDigest: contract.ReceiptV2Digest(receipt), ConfirmationMAC: code()})
				}))
				if when == "begin-marker" {
					inject()
				}
				challenge, err := j.BeginWorkflowDATA(context.Background(), func() SourceConfig { return cfg }, workflowLimits())
				if when == "submit-marker" || when == "receipt-pin" {
					if err != nil {
						t.Fatal("preparation failed", err)
					}
					if when == "submit-marker" {
						inject()
					}
					var response [32]byte
					response[0] = 1
					receipt, submitErr := j.SubmitWorkflowDATA(context.Background(), challenge.ID, response, func() SourceConfig { return cfg })
					err = submitErr
					if receipt != (contract.WorkflowReceiptWire{}) {
						t.Fatal("failed save returned receipt")
					}
				} else if challenge != (contract.Challenge{}) {
					t.Fatal("failed save returned challenge")
				}
				if err == nil {
					t.Fatal("fault succeeded")
				}
				uncertain := stage == "after-rename" || stage == "dirsync"
				if uncertain && !errors.Is(err, ErrUncertain) {
					t.Fatal("lost local uncertainty", err)
				}
				expectedSends := 0
				switch when {
				case "challenge-pin", "submit-marker":
					expectedSends = 1
				case "receipt-pin":
					expectedSends = 2
				}
				if sends != expectedSends {
					t.Fatal("network before intent or replay", sends, expectedSends)
				}
				if when == "begin-marker" && !uncertain {
					after, _ := os.ReadFile(j.path)
					if !bytes.Equal(original, after) {
						t.Fatal("rollback-safe original v2 changed")
					}
				}
				j.Close()
				reopen, err := Open(dir)
				if err != nil {
					t.Fatal("whole crash image refused", err)
				}
				defer reopen.Close()
				phase, _ := reopen.Status()
				expected := "exchanged-data"
				switch when {
				case "begin-marker":
					if uncertain {
						expected = "workflow-begin-may-have-started"
					}
				case "challenge-pin":
					expected = "workflow-begin-may-have-started"
					if uncertain {
						expected = "workflow-challenge-data"
					}
				case "submit-marker":
					expected = "workflow-challenge-data"
					if uncertain {
						expected = "workflow-submit-may-have-started"
					}
				case "receipt-pin":
					expected = "workflow-submit-may-have-started"
					if uncertain {
						expected = "workflow-receipt-data"
					}
				}
				if phase != expected {
					t.Fatal("wrong whole-image crash phase", phase, expected)
				}
				if _, err := reopen.BeginWorkflowDATA(context.Background(), func() SourceConfig { return cfg }, workflowLimits()); err == nil {
					t.Fatal("owning reopen repeated begin")
				}
			})
		}
	}
}

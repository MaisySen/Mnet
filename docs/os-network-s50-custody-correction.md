# S50 correction delta, S49 RED retained

S49 independent batch was RED204/1: the source-text lineage oracle opened removed
custody_linux.go and searched for the former c.file.Close spelling. Parent also
reported targeted race10 and live compile pass, which do not erase that RED or
prove kernel behavior. No author tests/race/run/device/helper/read/open/close.

## Corrections

- Source-text oracle now inspects file_custody.go Close and requires
  lineage.invalidate to precede the actual closer.Close call. Existing behavioral
  TestOriginalCloseInvalidatesBeforeCloserEvenOnError remains unchanged. No guard,
  once/error behavior or behavioral invalidation assertion was weakened.
- Removed unused ownerless bindOriginalFD from production entirely. Successful
  pure creation-binding test uses actual bindOriginalCustody and fake closer,
  so no test-only compatibility helper was needed. Live creation still requires
  a published controller-owned attempt before open.
- Repeated stop records the original sticky close error only once rather than
  joining the same error on every call, and returns that stored error for failed
  close after clean fake absence. Test additionally checks error object remains
  identical across repeated stop. Original closer invocation remains once.
- Renamed/collapsed publication test to
  TestAfterOpenPublishesCustodyBeforeInjectedCheck. It asserts ONLY shared afterOpen
  publication-before-one-injected-error ordering and retained owed close, not
  five real post-open/ioctl/readback/handle/session failure paths. Fake stage names
  did not establish those paths and have been removed.
- Added actual pure bindOriginalCustody mismatch test: observer returns link whose
  alias differs from AttachCreated; function refuses, returns same retained
  custody, leaves optional handle nil and does not close early. Existing actual
  observer-error bind test remains. This covers those pure function branches,
  not live readback or session binding failure.
- Renamed fake rollback-foreign mode to foreign-handle-ignored. It supplies an
  unrelated Handle to controller.stop and proves original close is not redirected
  or blocked. It DOES NOT call or test Rollback. Stop deliberately bypasses it.

## Evidence limits, corrected claims

Immediate production opener os.File wrapping, afterOpen placement before fallible
checks and removal of ORIGINAL raw-fd failure-defer close are SOURCE-READ plus
compile evidence only. No real open, TUNSETIFF, raw readback, namespace inspection,
close or first production caller was authorized or executed. The afterOpen fake
seam cannot prove a live failure follows that source order at runtime. The actual
pure bind tests cannot prove kernel original-link identity or session binding.

S49's modeled stage test was described too broadly as failure-stage coverage;
this correction narrows it to what its body did. S49 docs/history and failed
batch are retained, not rewritten as a green result. Operational Snapshot,
production absence, Create/app wiring and ops2-5 stay held. Ready unchanged.

Compile-only source check uses unchanged preinstalled Go1.26.11 offline explicit
package/development+process_launch tags, CGO off, no tests/run/binary/preflight.
Log reports success for production source. Subsequent changes in this correction
were test-only and this note; no claim that go build compiled those tests.
Independent once-only corrected inert review remains parent-owned.

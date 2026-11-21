package enrollmentpending

// Strict safety-only admission for a separate inactive native recovery owner.
// Exact file/projection equality does NOT confer authenticated delivery readiness.
// Caller must own/revalidate the existing native State and policy byte revisions.
func preparedRecoveryMatches(r record, originalRevision, policyRevision, device string, cfg SourceConfig) bool {
	if !(((r.Version == 5 || r.Version == 7) && r.Phase == "native-finalization-prepared") || (r.Version == 8 && (r.Phase == "history-lookup-may-have-started" || r.Phase == "history-receipt-data" || r.Phase == "native-finalization-prepared"))) {
		return false
	}
	if !validRecord(r) || !historyPolicyCurrent(r, cfg, device) {
		return false
	}
	f, ok := decodeFinalization(r)
	return ok && f.OriginalStateRevision == originalRevision && f.PolicyRevision == policyRevision
}

// CheckPreparedRecoveryStateDATA exposes no credential or readiness and never
// rewrites a file. Missing/completed/malformed/wrong-device/revision is refused.
func CheckPreparedRecoveryStateDATA(raw []byte, originalRevision, policyRevision, device string, cfg SourceConfig) bool {
	if !token(originalRevision) || !token(policyRevision) {
		return false
	}
	var r record
	return strictRecord(raw, &r) && preparedRecoveryMatches(r, originalRevision, policyRevision, device, cfg)
}

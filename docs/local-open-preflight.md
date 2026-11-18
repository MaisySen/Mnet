# Local stream OPEN metadata preflight

The wire writer already refuses OPEN metadata larger than16KiB. Dynamic Open previously marshaled and queued such metadata in a running mux, returned success, then allowed the asynchronous writer to discover the error and close the whole session. The same local mistake before the writer started could enter synchronous socket I/O before the size error was reported.

Dynamic Open now checks serialized metadata against the existing limit before allocating a stream ID, creating stream state or enqueueing. The caller gets the existing OPEN-policy error directly; the unrelated active mux stays usable. The wire limit and valid16KiB boundary are unchanged. Metadata still must be serialized before its size is known, so this is not an allocation limit on arbitrary caller-supplied objects.

Two test families pass 30 fresh local race repetitions. The first checks stopped/started writer preflight with no ID/table/queue/marker changes or session termination. The second uses a real framed net.Pipe session: an oversized local Open is refused, then an exactly16KiB serialized Open and reply succeed on the same session. A source overlay restoring the old implementation reproduces oversized live Open returning success. Full uncached repository race tests and vet pass locally. Desktop CI includes both families at30repetitions; exact-revision results need collection.

No active-session queue limits, wire protocol, consumer consent/destination/retention policy, Multies integration, native Android experiment, signing/account/device action or release changes. Tests use disposable metadata and local fixtures, not user content or live sessions.

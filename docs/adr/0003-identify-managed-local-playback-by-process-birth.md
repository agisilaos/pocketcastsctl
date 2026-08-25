# Identify managed local playback by process birth

pocketcastsctl identifies managed local playback with a PID paired with the Darwin kernel process-start time and verifies that identity around every signal. Lifecycle operations remain short-lived and serialize through an interprocess lock rather than a supervisor daemon. Legacy records without a verifiable birth identity are invalidated without signaling, accepting possible unmanaged pre-upgrade playback and a small verification-to-signal race in exchange for avoiding adoption of an unrelated reused PID and keeping playback daemon-free.

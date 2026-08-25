# Environment overrides are not replaceable

Authentication replacement is decided from the resolved active API session and its credential source, never from saved metadata alone. When an environment override is active, commands that persist a new API session refuse to proceed even with `--force` and direct the user to remove the override first, because otherwise they could report success while only changing a dormant saved session. This preserves truthful replacement semantics at the cost of requiring scripts that intentionally update dormant credentials to run without the override.

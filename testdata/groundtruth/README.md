# Ground-truth secret corpus

This corpus is synthetic and exists only to measure detector recall and false
positives. It is not a set of live credentials.

`manifest.json` is the source of truth. Every positive case must be excluded by
the union; every benign case must not be excluded. The test lives in
`internal/detect/groundtruth_test.go` and materializes the corpus into a
temporary directory before scanning, so the repository is never modified.

The PEM case is generated at test time with `crypto/rsa` (2048-bit PKCS#8) and
is labeled `"generate": "rsa2048"` in the manifest. No private key material is
committed. The placeholder `positive/private_key.properties` only records that
the case exists.

Positive files deliberately use `.yaml`, `.json`, `.toml`, `.ini`, `.cfg`, and
`.properties` extensions so they exercise the U0 second pass (`needsSecondPass`)
without matching sensitive filename rules such as `*.pem` or `.env`. That makes
the test measure content detection rather than path-based sensitivity.

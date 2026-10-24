# Test databases

The `.mmdb` files in this directory are the MaxMind test databases from the
[maxmind/MaxMind-DB](https://github.com/maxmind/MaxMind-DB) repository, directory
`test-data/`, at commit `255b9a8b8a692d70dcaef31dc2ca44fa7ce50ece`. They contain
fabricated data for a small set of networks and are intended for testing only.

| file | sha256 |
|------|--------|
| `GeoIP2-City-Test.mmdb` | `1394af009121820d43b620fea19fc4b6f09141698091c00f70f225a982389035` |
| `GeoIP2-Country-Test.mmdb` | `fc12d6470b35b70bd3e4534dd433d3e262a6c73f7682f85c344e832d4190f02b` |
| `GeoLite2-ASN-Test.mmdb` | `a0f9e8614ad582ba1af3eeb18ead666f73ad6e4cbd5280f80d5d4d95bbe6c3f1` |

The expected values asserted by the tests come from the matching
`source-data/*.json` files of the same commit.

## Licence

The MaxMind-DB repository, including its test data, is
Copyright (c) 2013 - 2026 by MaxMind, Inc. and is distributed under the
Apache License, Version 2.0 or the MIT License, at your option
(see `LICENSE-APACHE` and `LICENSE-MIT` in that repository). The files here are
redistributed unmodified under the MIT License option.

Updating: download the three files from the `test-data/` directory of a newer
commit, update the commit hash and checksums above, and re-check the expected
values in `middleware_test.go` against `source-data/`.

# Security Policy

## Supported Versions

go-milter has not reached 1.0 yet. Security fixes are only made for the latest minor release.

| Version  | Supported |
|----------|-----------|
| 0.10.x   | yes       |
| < 0.10   | no        |

## Reporting a Vulnerability

Please do **not** report security vulnerabilities through public GitHub issues, discussions, or pull requests.

Instead, report them privately using one of these channels:

1. **GitHub private vulnerability reporting (preferred):**
   open a report at <https://github.com/d--j/go-milter/security/advisories/new>.
2. **E-Mail:** send your report to [daniel@jagszent.de](mailto:daniel@jagszent.de).
   If possible, encrypt it with the GPG key [F28F4D2FA2C168E1](https://keys.openpgp.org/search?q=F28F4D2FA2C168E1)
   (Fingerprint: 60C4 D61A EA92 CC54 AE79 559B F28F 4D2F A2C1 68E1)

Please include as much of the following as you can:

- the affected version(s) or commit(s)
- the affected component (e.g. milter server, client, `mailfilter`, `milterutil`)
- a description of the issue and its impact (e.g. what a malicious MTA or milter peer can achieve)
- steps to reproduce, ideally a minimal proof of concept or a failing test
- any known workarounds

## What to Expect

- You will receive an acknowledgement of your report.
- The report is investigated and you are kept informed about the progress.
- Once a fix is available, a new release is published together with a GitHub security advisory.
  You will be credited in the advisory unless you prefer to stay anonymous.
  We assume that you want to be credited unless you explicitly request anonymity.

Please give us a reasonable amount of time to fix the issue before disclosing it publicly.

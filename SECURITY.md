# Security Policy for Chameleon

The Chameleon team and community take security bugs in Chameleon seriously. We appreciate your efforts to responsibly disclose your findings, and will make every effort to acknowledge your contributions.

## Reporting a Vulnerability

We are committed to working with the community to verify and respond to any potential vulnerabilities that are reported to us.

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please report them directly to us via email at:

**[chameleon@sequre42.com]** 

Please include the following details with your report:

*   A clear description of the vulnerability.
*   The version of Chameleon affected (e.g., a specific commit hash, tag, or release version).
*   Steps to reproduce the vulnerability. This is crucial for us to verify the issue.
*   If possible, include any proof-of-concept code, scripts, or screenshots.
*   Any potential impact of the vulnerability.
*   Your name or alias for acknowledgment (if you wish to be credited).

## Disclosure Policy

*   Once a security report is received, we will acknowledge receipt within [e.g., 48 hours / 2 business days].
*   We will investigate the reported vulnerability and work to validate it.
*   We will maintain an open dialogue with you during the investigation process.
*   If the vulnerability is confirmed, we will work on a fix and plan for a release.
*   We aim to disclose the vulnerability publicly once a fix is available, coordinating with you on the timing if appropriate.
*   We will credit you for your discovery, unless you prefer to remain anonymous.

## Scope

This security policy applies to the latest stable release of Chameleon and any actively supported development branches. If you believe a vulnerability affects older, unsupported versions, please still report it, but our primary focus will be on current versions.

## Out of Scope

The following are generally considered out of scope for our vulnerability disclosure program (unless they lead to a direct, exploitable vulnerability in Chameleon itself):

*   Vulnerabilities in upstream SOCKS5 proxies that Chameleon might connect to (these should be reported to the maintainers of those proxies).
*   Vulnerabilities in the Go language runtime or standard libraries (these should be reported to the Go security team).
*   Denial of service attacks that require overwhelming network traffic (standard DoS/DDoS).
*   Social engineering or phishing attempts.
*   Issues related to the security of your own server environment where Chameleon is deployed (e.g., OS hardening, firewall rules).

## Questions

If you have any questions regarding this security policy, please contact us at the email address provided above.

Thank you for helping keep Chameleon secure!
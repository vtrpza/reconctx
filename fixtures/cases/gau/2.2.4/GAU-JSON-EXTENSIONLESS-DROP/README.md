# GAU-JSON-EXTENSIONLESS-DROP

Regression fixture for GAU v2.2.4. `--json` drops every URL whose path has no extension. The provider set returned three records, all extensionless, while the native JSON output remained empty and the process exited 0. The released source lacks the `ext != ""` guard present upstream after v2.2.4.

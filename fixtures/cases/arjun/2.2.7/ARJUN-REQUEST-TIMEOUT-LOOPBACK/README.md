# ARJUN-REQUEST-TIMEOUT-LOOPBACK

Against a deterministic 5-second loopback response with `-T 1`, Arjun 2.2.7 exited 1 before the outer safety timeout and raised `AttributeError: 'str' object has no attribute 'status_code'`. This fixture is a tool-error case; it supports no parameter-absence claim.

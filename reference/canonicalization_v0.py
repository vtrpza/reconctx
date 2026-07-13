"""Executable reference for URL and identity canonicalization v0.

This module is a discovery artifact, not the production adapter implementation.
Raw source values must always be preserved alongside derived values.
"""

from __future__ import annotations

import hashlib
import ipaddress
import re
import unicodedata
from urllib.parse import urlsplit

import idna


_UNRESERVED = frozenset(
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
)
_PATH_SAFE = _UNRESERVED | frozenset("/:@!$&'()*+,;=")
_QUERY_COMPONENT_SAFE = _UNRESERVED | frozenset("/:?@!$'()*+,;=")
_METHOD_TOKEN = re.compile(r"^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")
_ENDPOINT_ID = re.compile(r"^ep_sha256_[0-9a-f]{64}$")
_PARAMETER_LOCATIONS = {
    "query",
    "form",
    "json",
    "header",
    "cookie",
    "path",
    "unknown",
}


class CanonicalizationError(ValueError):
    """Raised when a URL cannot be canonicalized without ambiguity."""


def _normalize_percent_component(value: str, safe: frozenset[str], label: str) -> str:
    value = unicodedata.normalize("NFC", value)
    result: list[str] = []
    index = 0
    while index < len(value):
        char = value[index]
        if char == "%":
            if index + 2 >= len(value):
                raise CanonicalizationError(f"invalid percent escape in {label}")
            pair = value[index + 1 : index + 3]
            if not all(item in "0123456789abcdefABCDEF" for item in pair):
                raise CanonicalizationError(f"invalid percent escape in {label}")
            byte = int(pair, 16)
            decoded = chr(byte)
            if byte < 128 and decoded in _UNRESERVED:
                result.append(decoded)
            else:
                result.append(f"%{byte:02X}")
            index += 3
            continue
        if char in safe:
            result.append(char)
        else:
            result.extend(f"%{byte:02X}" for byte in char.encode("utf-8"))
        index += 1
    return "".join(result)


def _remove_last_segment(output: str) -> str:
    slash = output.rfind("/")
    return output[:slash] if slash >= 0 else ""


def _remove_dot_segments(path: str) -> str:
    """RFC 3986 section 5.2.4 without collapsing repeated slashes."""
    input_buffer = path
    output = ""
    while input_buffer:
        if input_buffer.startswith("../"):
            input_buffer = input_buffer[3:]
        elif input_buffer.startswith("./"):
            input_buffer = input_buffer[2:]
        elif input_buffer.startswith("/./"):
            input_buffer = "/" + input_buffer[3:]
        elif input_buffer == "/.":
            input_buffer = "/"
        elif input_buffer.startswith("/../"):
            input_buffer = "/" + input_buffer[4:]
            output = _remove_last_segment(output)
        elif input_buffer == "/..":
            input_buffer = "/"
            output = _remove_last_segment(output)
        elif input_buffer in {".", ".."}:
            input_buffer = ""
        else:
            if input_buffer.startswith("/"):
                next_slash = input_buffer.find("/", 1)
            else:
                next_slash = input_buffer.find("/")
            if next_slash == -1:
                output += input_buffer
                input_buffer = ""
            else:
                output += input_buffer[:next_slash]
                input_buffer = input_buffer[next_slash:]
    return output


def _nonstandard_numeric_host(host: str) -> bool:
    labels = host.split(".")
    return all(
        label.isdigit()
        or (
            label.lower().startswith("0x")
            and len(label) > 2
            and all(char in "0123456789abcdefABCDEF" for char in label[2:])
        )
        for label in labels
    )


def _canonical_host(host: str) -> tuple[str, bool]:
    host = unicodedata.normalize("NFC", host).rstrip(".")
    if not host or "%" in host or host[0] in ".。．｡":
        raise CanonicalizationError("invalid or empty host")
    try:
        ip = ipaddress.ip_address(host)
    except ValueError:
        if _nonstandard_numeric_host(host):
            raise CanonicalizationError("non-standard numeric IP address is forbidden")
        try:
            encoded = idna.encode(
                host,
                uts46=True,
                transitional=False,
                std3_rules=True,
            ).decode("ascii")
        except idna.IDNAError as exc:
            raise CanonicalizationError(f"invalid IDNA host: {exc}") from exc
        encoded = encoded.rstrip(".")
        if not encoded or encoded.startswith(".") or ".." in encoded:
            raise CanonicalizationError("invalid IDNA host")
        try:
            ipaddress.ip_address(encoded)
        except ValueError:
            pass
        else:
            raise CanonicalizationError("mapped numeric IP address is forbidden")
        if _nonstandard_numeric_host(encoded):
            raise CanonicalizationError("non-standard numeric IP address is forbidden")
        return encoded.lower(), False
    return ip.compressed.lower(), ip.version == 6


def _query_pairs(query: str) -> tuple[list[dict], str]:
    if query == "":
        return [], ""
    records: list[dict] = []
    encoded_parts: list[str] = []
    for index, component in enumerate(query.split("&")):
        has_equals = "=" in component
        if has_equals:
            raw_name, raw_value = component.split("=", 1)
            value: str | None = _normalize_percent_component(
                raw_value, _QUERY_COMPONENT_SAFE, "query value"
            )
        else:
            raw_name = component
            raw_value = None
            value = None
        name = _normalize_percent_component(
            raw_name, _QUERY_COMPONENT_SAFE, "query name"
        )
        encoded_parts.append(name + ("=" + value if has_equals else ""))
        records.append(
            {
                "index": index,
                "raw_name": raw_name,
                "raw_value": raw_value,
                "name": name,
                "value": value,
                "has_equals": has_equals,
            }
        )
    return records, "&".join(encoded_parts)


def canonicalize_url(raw_url: str) -> dict:
    """Return loss-aware HTTP URL canonical forms for v0."""
    if not isinstance(raw_url, str) or not raw_url:
        raise CanonicalizationError("URL must be a non-empty string")
    if "\\" in raw_url or any(ord(char) < 0x20 or ord(char) == 0x7F for char in raw_url):
        raise CanonicalizationError("URL contains ambiguous slash or control characters")
    try:
        parsed = urlsplit(raw_url)
    except ValueError as exc:
        raise CanonicalizationError(str(exc)) from exc
    scheme = parsed.scheme.lower()
    if scheme not in {"http", "https"}:
        raise CanonicalizationError("only absolute HTTP(S) URLs are supported")
    if not parsed.netloc or parsed.hostname is None:
        raise CanonicalizationError("absolute URL authority is required")
    if parsed.username is not None or parsed.password is not None or "@" in parsed.netloc:
        raise CanonicalizationError("userinfo is forbidden")
    host, is_ipv6 = _canonical_host(parsed.hostname)
    try:
        port = parsed.port
    except ValueError as exc:
        raise CanonicalizationError(str(exc)) from exc
    if port is not None and not 1 <= port <= 65535:
        raise CanonicalizationError("port out of range")
    if (scheme == "http" and port == 80) or (scheme == "https" and port == 443):
        port = None

    authority_host = f"[{host}]" if is_ipv6 else host
    authority = authority_host + (f":{port}" if port is not None else "")
    origin = f"{scheme}://{authority}"

    normalized_path = _normalize_percent_component(
        parsed.path or "/", _PATH_SAFE, "path"
    )
    path = _remove_dot_segments(normalized_path) or "/"
    if not path.startswith("/"):
        raise CanonicalizationError("HTTP URL path must be absolute")

    before_fragment = raw_url.split("#", 1)[0]
    query_present = "?" in before_fragment
    pairs, canonical_query = _query_pairs(parsed.query) if query_present else ([], "")
    route = origin + path
    observation = route + ("?" + canonical_query if query_present else "")
    fragment_present = "#" in raw_url
    if fragment_present:
        _normalize_percent_component(parsed.fragment, _QUERY_COMPONENT_SAFE, "fragment")

    return {
        "raw_url": raw_url,
        "scheme": scheme,
        "host": host,
        "port": port,
        "origin": origin,
        "path": path,
        "query_present": query_present,
        "query_raw": parsed.query if query_present else None,
        "query_canonical": canonical_query if query_present else None,
        "query_pairs": pairs,
        "fragment_present": fragment_present,
        "fragment_raw": parsed.fragment if fragment_present else None,
        "canonical_route_url": route,
        "canonical_observation_url": observation,
        "warnings": ["fragment_removed"] if fragment_present else [],
    }


def _http_method(method: str) -> str:
    if not isinstance(method, str) or not _METHOD_TOKEN.fullmatch(method):
        raise CanonicalizationError("invalid HTTP method token")
    return method.upper()


def normalize_source_method(source_label: str | None, tool: str | None = None) -> dict:
    tool_name = (tool or "").lower()
    if source_label is None:
        return {
            "source_label": None,
            "http_method": None,
            "method_known": False,
            "body_kind": "unknown",
            "parameter_location": "unknown",
        }
    label = source_label.upper()
    if tool_name == "arjun":
        mapping = {
            "GET": ("GET", "none", "query"),
            "POST": ("POST", "form", "form"),
            "JSON": ("POST", "json", "json"),
        }
        if label not in mapping:
            raise CanonicalizationError(f"unsupported Arjun method mode: {source_label}")
        method, body_kind, location = mapping[label]
        return {
            "source_label": label,
            "http_method": method,
            "method_known": True,
            "body_kind": body_kind,
            "parameter_location": location,
        }
    method = _http_method(source_label)
    return {
        "source_label": source_label,
        "http_method": method,
        "method_known": True,
        "body_kind": "unknown",
        "parameter_location": "unknown",
    }


def _stable_hash(prefix: str, *components: str) -> str:
    material = "\0".join((f"reconctx-{prefix}-v0", *components)).encode("utf-8")
    return f"{prefix}_sha256_{hashlib.sha256(material).hexdigest()}"


def endpoint_id(method: str | None, raw_or_route_url: str) -> str:
    route = canonicalize_url(raw_or_route_url)["canonical_route_url"]
    canonical_method = "*" if method is None else _http_method(method)
    if method is not None and canonical_method == "*":
        canonical_method = r"\*"
    return _stable_hash("ep", canonical_method, route)


def parameter_id(endpoint: str, location: str, name: str) -> str:
    if not _ENDPOINT_ID.fullmatch(endpoint):
        raise CanonicalizationError("invalid endpoint ID")
    if location not in _PARAMETER_LOCATIONS:
        raise CanonicalizationError("invalid parameter location")
    normalized_name = unicodedata.normalize("NFC", name)
    if not normalized_name:
        raise CanonicalizationError("parameter name cannot be empty")
    return _stable_hash("param", endpoint, location, normalized_name)

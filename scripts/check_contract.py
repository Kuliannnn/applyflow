"""Validate OpenAPI plus local invariants. Never fetch external schema references."""
from pathlib import Path
import re
import yaml
from jsonschema import Draft202012Validator, FormatChecker
from openapi_spec_validator import validate

ROOT = Path(__file__).resolve().parents[1]


class UniqueKeyLoader(yaml.SafeLoader):
    """Duplicate YAML keys must fail rather than silently delete an endpoint."""


def unique_mapping(loader, node, deep=False):
    result = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node, deep=deep)
        if key in result:
            raise ValueError(f"Duplicate YAML key: {key!r} at line {key_node.start_mark.line + 1}")
        result[key] = loader.construct_object(value_node, deep=deep)
    return result


UniqueKeyLoader.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, unique_mapping)


def load_spec():
    return yaml.load((ROOT / "api/openapi.yaml").read_text(), Loader=UniqueKeyLoader)


def schema_validator(spec, name):
    # Keep the original local OpenAPI component references resolvable by JSON Schema.
    return Draft202012Validator(
        {"$ref": f"#/components/schemas/{name}", "components": spec["components"]},
        format_checker=FormatChecker(),
    )


def check(spec):
    def refs(node):
        if isinstance(node, dict):
            if "$ref" in node:
                pointer = node["$ref"]
                assert pointer.startswith("#/"), f"External ref not permitted: {pointer}"
                target = spec
                for part in pointer[2:].split("/"):
                    target = target[part.replace("~1", "/").replace("~0", "~")]
            for value in node.values(): refs(value)
        elif isinstance(node, list):
            for value in node: refs(value)
    refs(spec)
    validate(spec)
    ids = set()
    count = 0
    for path, methods in spec["paths"].items():
        for method, operation in methods.items():
            if method not in {"get", "post", "put", "patch", "delete"}: continue
            assert operation["operationId"] not in ids, "Duplicate operationId"
            ids.add(operation["operationId"])
            count += 1
            params = operation.get("parameters", [])
            assert set(re.findall(r"\{([^}]+)\}", path)) == {p["name"] for p in params if p["in"] == "path"}
            if not path.startswith("/api/auth/"):
                assert operation.get("security", spec["security"]), f"Unprotected endpoint: {path}"
            if method != "get":
                assert any(p["name"] == "X-CSRF-Token" and p["required"] for p in params), path
            for code, response in operation["responses"].items():
                if code.startswith("2"):
                    assert response["headers"]["Cache-Control"]["$ref"] == "#/components/headers/Cache-Control"
                    assert spec["components"]["headers"]["Cache-Control"]["schema"]["const"] == "no-store"
    for name, schema in spec["components"]["schemas"].items():
        if "example" in schema: schema_validator(spec, name).validate(schema["example"])
    return count


if __name__ == "__main__":
    print(f"OpenAPI and contract invariants valid: {check(load_spec())} operations")

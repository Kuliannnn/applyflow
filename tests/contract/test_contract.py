"""Consumer-facing regressions: reject identity injection and stale/unusable edits."""
import sys
from pathlib import Path
import unittest
from jsonschema import ValidationError

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "scripts"))
from check_contract import check, load_spec, schema_validator


class ContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.spec = load_spec()

    def valid(self, schema, payload):
        schema_validator(self.spec, schema).validate(payload)

    def invalid(self, schema, payload):
        with self.assertRaises(ValidationError): self.valid(schema, payload)

    def test_complete_spec(self):
        check(self.spec)

    def test_cannot_assign_owner_or_versions_on_create(self):
        base = {"company": "Example", "role_title": "Engineer"}
        self.valid("ApplicationCreate", base)
        for key in ["owner_id", "user_id", "version", "jd_version", "archived_at"]:
            self.invalid("ApplicationCreate", {**base, key: "injected"})

    def test_update_requires_version_and_actual_fields(self):
        self.invalid("ApplicationPatch", {"status": "applied"})
        self.invalid("ApplicationPatch", {"expected_version": 1})
        self.invalid("ApplicationPatch", {"expected_version": 0, "notes": "x"})
        self.valid("ApplicationPatch", {"expected_version": 1, "job_url": None})
        self.invalid("ApplicationPatch", {"expected_version": 1, "company": None})

    def test_profile_cannot_change_login_or_other_sections(self):
        self.valid("ProfilePatch", {"expected_version": 1, "basics": {"contact_email": None}})
        self.invalid("ProfilePatch", {"expected_version": 1, "basics": {}})
        self.invalid("ProfilePatch", {"expected_version": 1, "basics": {"email": "x@example.com"}})
        self.invalid("ProfilePatch", {"expected_version": 1, "preferences": {}})

    def test_skill_and_status_enums(self):
        self.invalid("UserSkillWrite", {"expected_version": 1, "proficiency": "not_assessed"})
        self.invalid("ApplicationPatch", {"expected_version": 1, "status": "interview"})
        self.valid("UserSkillWrite", {"expected_version": 1, "proficiency": "learning"})

    def test_reject_invalid_dates_and_non_https_urls(self):
        self.invalid("ApplicationCreate", {"company": "A", "role_title": "B", "date_applied": "2026-02-30"})
        self.invalid("ProfilePatch", {"expected_version": 1, "basics": {"website_url": "javascript:alert(1)"}})


if __name__ == "__main__": unittest.main()

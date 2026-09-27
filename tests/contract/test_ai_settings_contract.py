"""Public credential DTOs must never accept ownership or echo secrets."""
import unittest
import sys
from pathlib import Path
from jsonschema import ValidationError

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
from check_contract import load_spec, schema_validator


class AISettingsContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.spec = load_spec()

    def test_write_only_secret_and_strict_shapes(self):
        put = schema_validator(self.spec, 'AIConfigPut')
        body = dict(expected_version=0, provider_id='openai', model_id='gpt-4.1-mini', daily_request_limit=20)
        put.validate(body)  # Stored-key existence is a database precondition.
        put.validate(dict(body, api_key='test-only-placeholder'))
        for extra in [dict(api_key=None), dict(api_key=''), dict(api_key='********'), dict(api_key='a\nsecret'), dict(owner_id='other'), dict(base_url='http://localhost'), dict(model_id='unknown'), dict(daily_request_limit=101)]:
            with self.subTest(extra=extra), self.assertRaises(ValidationError):
                put.validate(dict(body, **extra))
        self.assertTrue(self.spec['components']['schemas']['AIConfigPut']['properties']['api_key']['writeOnly'])
        self.assertNotIn('api_key', self.spec['components']['schemas']['AIConfig']['properties'])

    def test_patch_requires_change_and_rejects_null(self):
        patch = schema_validator(self.spec, 'AIConfigPatch')
        patch.validate(dict(expected_version=1, enabled=False))
        for body in [dict(expected_version=1), dict(expected_version=1, enabled=None), dict(expected_version=1, mode='platform')]:
            with self.assertRaises(ValidationError):
                patch.validate(body)

    def test_completed_and_in_flight_test_responses(self):
        test = self.spec['paths']['/api/me/ai-config/test']['post']
        self.assertIn('200', test['responses'])
        self.assertIn('202', test['responses'])
        self.assertTrue(any(p['name']=='Idempotency-Key' and p['required'] for p in test['parameters']))
        schema_validator(self.spec, 'AIConfigTest').validate(dict(expected_version=0, revision=1))
        with self.assertRaises(ValidationError):
            schema_validator(self.spec, 'AIConfigTest').validate(dict(expected_version=0, revision=0))

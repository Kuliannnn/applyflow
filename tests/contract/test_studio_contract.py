"""Studio consumer constraints; authorization is also verified by HTTP integration tests."""
import copy
import unittest
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "scripts"))
from check_contract import schema_validator
from jsonschema import ValidationError

ID = '20000000-0000-4000-8000-000000000001'


class StudioContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        from check_contract import load_spec
        cls.spec = load_spec()

    def valid(self, name, body):
        schema_validator(self.spec, name).validate(body)

    def invalid(self, name, body):
        with self.assertRaises(ValidationError): self.valid(name, body)

    def test_source_union_rejects_ambiguous_and_empty_input(self):
        for source in [{'kind':'text','text':'Go engineer'}]:
            self.valid('SourceCreate', {'expected_version':1,'source':source})
        for source in [{'kind':'url','url':'https://example.com/job'}, {'kind':'image','file_ids':[ID]}, {'kind':'text','text':' '}, {'kind':'text','text':'JD','url':'https://example.com'}, {'kind':'url','url':'http://example.com'}, {'kind':'image','file_ids':[]}, {'kind':'image','file_ids':[ID,ID]}, {'kind':'image','file_ids':[ID]*6}]:
            self.invalid('SourceCreate', {'expected_version':1,'source':source})

    def test_owner_and_storage_injection_rejected(self):
        cases={'WorkspaceCreate':{}, 'ResumeImport':{'file_id':ID,'name':'Base','import_mode':'manual'}, 'GenerationCreate':self.spec['components']['schemas']['GenerationCreate']['example']}
        for name,body in cases.items():
            self.valid(name,body)
            for key in ['owner_id','storage_key','status','fencing_token','api_key']:
                self.invalid(name,{**body,key:ID})

    def test_manual_import_is_explicit_and_does_not_claim_a_parser(self):
        self.valid('ResumeImport', {'file_id':ID,'name':'Base','import_mode':'manual'})
        self.invalid('ResumeImport', {'file_id':ID,'name':'Base'})
        self.invalid('ResumeImport', {'file_id':ID,'name':'Base','import_mode':'automatic'})
        op=self.spec['paths']['/api/resumes']['post']
        self.assertIn('201',op['responses'])
        self.assertNotIn('202',op['responses'])
        self.assertEqual(self.spec['components']['schemas']['ResumeImported']['properties']['task_id']['type'],'null')

    def test_edits_require_versions_and_real_changes(self):
        self.invalid('WorkspacePatch',{'expected_version':1})
        self.invalid('WorkspacePatch',{'title':'New'})
        self.invalid('WorkspacePatch',{'expected_version':0,'title':'New'})
        self.valid('WorkspacePatch',{'expected_version':1,'resume_revision_id':None})
        self.invalid('ResumePatch',{'expected_version':1})
        self.valid('CandidateApply',{'expected_version':2,'revision_id':ID})
        self.invalid('CandidateApply',{'revision_id':ID})

    def test_generation_pins_sources_and_requires_personal_revision(self):
        body=copy.deepcopy(self.spec['components']['schemas']['GenerationCreate']['example'])
        self.valid('GenerationCreate',body)
        for key in body:
            bad=body.copy();bad.pop(key);self.invalid('GenerationCreate',bad)
        self.invalid('GenerationCreate',{**body,'execution_mode':'personal'})
        self.valid('GenerationCreate',{**body,'execution_mode':'personal','ai_revision':1})
        self.invalid('GenerationCreate',{**body,'ai_revision':1})
        self.invalid('GenerationCreate',{**body,'execution_mode':'personal','ai_revision':0})
        self.invalid('GenerationCreate',{**body,'locale':'zh'})

    def test_fact_confirmation_requires_evidence_and_nonempty_facts(self):
        body={'expected_version':1,'facts':[{'id':ID,'category':'experience','text':'Built APIs','evidence':{'source':'user','page':None,'excerpt':''}}]}
        self.valid('ResumeConfirm',body)
        self.invalid('ResumeConfirm',{'expected_version':1,'facts':[]})
        bad=copy.deepcopy(body);bad['facts'][0]['evidence']['page']=21;self.invalid('ResumeConfirm',bad)
        bad=copy.deepcopy(body);bad['facts'][0]['proficiency']='expert';self.invalid('ResumeConfirm',bad)

    def test_document_shapes_cannot_mix_letter_and_resume(self):
        body=copy.deepcopy(self.spec['components']['schemas']['DocumentSave']['example'])
        self.valid('DocumentSave',body)
        bad=copy.deepcopy(body);bad['content']['kind']='cover_letter';self.invalid('DocumentSave',bad)
        bad=copy.deepcopy(body);bad['content']['html']='<script>bad()</script>';self.invalid('DocumentSave',bad)
        bad=copy.deepcopy(body);bad.pop('base_revision_id');self.invalid('DocumentSave',bad)

    def test_export_is_fixed_revision_and_supported_format(self):
        body={'revision_id':ID,'format':'pdf','template_version':'1'}
        self.valid('ExportCreate', {**body,'template_version':'2'})
        self.valid('ExportCreate',body)
        for key,value in [('revision_id','latest'),('format','html'),('template_version','3')]:
            self.invalid('ExportCreate',{**body,key:value})

    def test_task_safe_view_does_not_expose_execution_input(self):
        body={'task_id':ID,'kind':'tailor_resume','status':'queued','stage':'queued','state_version':1,'attempts':0,'cancel_requested':False,'result':None,'error_code':None,'created_at':'2026-09-21T00:00:00Z','finished_at':None}
        self.valid('TaskSnapshot',body)
        for key in ['input_snapshot','credential_id','storage_key','lease_until','fencing_token']:
            self.invalid('TaskSnapshot',{**body,key:ID})

    def test_task_terminal_and_result_kind_are_consistent(self):
        body={'task_id':ID,'kind':'export_document','status':'completed','stage':'completed','state_version':3,'attempts':1,'cancel_requested':False,'result':{'kind':'export','export_id':ID,'file_id':ID},'error_code':None,'created_at':'2026-09-21T00:00:00Z','finished_at':'2026-09-21T00:01:00Z'}
        self.valid('TaskSnapshot',body)
        for fields in [{'result':None},{'cancel_requested':True},{'finished_at':None},{'stage':'running'},{'kind':'parse_resume'},{'status':'running'}]:
            self.invalid('TaskSnapshot',{**body,**fields})

    def test_durable_create_commands_require_idempotency(self):
        operations={'createWorkspace','createJobSource','importResume','createGeneration','retryGenerationDocument','refineDocument','createDocumentExport','trackWorkspace'}
        for methods in self.spec['paths'].values():
            for operation in methods.values():
                if operation['operationId'] in operations:
                    self.assertTrue(any(p['name']=='Idempotency-Key' and p.get('required') for p in operation['parameters']))
                    operations.remove(operation['operationId'])
        self.assertFalse(operations)


if __name__=='__main__': unittest.main()

import CodeMirror, { EditorView } from '@uiw/react-codemirror'
import { PostgreSQL, sql } from '@codemirror/lang-sql'

export default function SqlCodeMirror({ value, onChange, readOnly, labelId, errorId }: { value: string; onChange?: (value: string) => void; readOnly: boolean; labelId: string; errorId?: string }) {
  return <CodeMirror value={value} onChange={onChange} extensions={[sql({ dialect: PostgreSQL }), EditorView.contentAttributes.of({ 'aria-labelledby': labelId, ...(errorId ? { 'aria-describedby': errorId, 'aria-invalid': 'true' } : {}) })]} theme="dark" readOnly={readOnly} editable={!readOnly} minHeight="140px" aria-labelledby={labelId} aria-describedby={errorId} aria-invalid={errorId ? true : undefined} basicSetup={{ lineNumbers: true, foldGutter: false, highlightActiveLine: !readOnly }} />
}

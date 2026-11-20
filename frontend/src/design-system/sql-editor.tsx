import CodeMirror from '@uiw/react-codemirror'
import { PostgreSQL, sql } from '@codemirror/lang-sql'

export default function SqlCodeMirror({ value, onChange, readOnly, labelId }: { value: string; onChange?: (value: string) => void; readOnly: boolean; labelId: string }) {
  return <CodeMirror value={value} onChange={onChange} extensions={[sql({ dialect: PostgreSQL })]} theme="dark" readOnly={readOnly} editable={!readOnly} minHeight="140px" aria-labelledby={labelId} basicSetup={{ lineNumbers: true, foldGutter: false, highlightActiveLine: !readOnly }} />
}

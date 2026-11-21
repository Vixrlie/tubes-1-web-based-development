import type { ReactNode } from 'react'
export function Section({ title, children, description }: { title: string; children: ReactNode; description?: string }) { return <section className="catalog-section"><div className="section-heading"><h2>{title}</h2>{description && <p>{description}</p>}</div>{children}</section> }
export function Preview({ children }: { children: ReactNode }) { return <div className="catalog-preview">{children}</div> }


// metapi-go/data-table — ported from newapi
export const staticDataTableClassNames = {
  container: 'overflow-hidden rounded-md border',
  sectionContainer: 'border-border/60 rounded-lg',
  embeddedContainer: 'rounded-none border-0',
  compactTable: 'text-sm',
  compactHeaderRow: 'hover:bg-transparent',
  mutedHeaderRow:
    '[background-color:var(--table-header)] hover:[background-color:var(--table-header-hover)]',
  compactHeaderCell:
    'text-muted-foreground py-2 text-[10px] font-medium tracking-wider uppercase',
  compactHeaderCellRight:
    'text-muted-foreground py-2 text-right text-[10px] font-medium tracking-wider uppercase',
  compactCell: 'py-2.5',
  compactTopCell: 'py-2.5 align-top',
  compactTopNumericCell: 'py-2.5 text-right align-top font-mono',
  compactMutedCell: 'text-muted-foreground py-2.5',
  compactMutedCodeCell: 'text-muted-foreground py-2.5 font-mono',
  compactNumericCell: 'py-2.5 text-right font-mono',
  compactMutedNumericCell: 'text-muted-foreground py-2.5 text-right font-mono',
  topCell: 'py-2 align-top',
  topMutedCell: 'text-muted-foreground py-2 align-top',
  codeCell: 'font-mono text-sm',
  mutedCell: 'text-muted-foreground text-sm',
  mutedCodeCell: 'text-muted-foreground font-mono text-sm',
  topNumericCell: 'py-2 text-right font-mono',
  mediumCell: 'font-medium',
  actionHeaderCell: 'w-auto max-w-none text-right',
  actionCell: 'w-auto max-w-none text-right',
} as const

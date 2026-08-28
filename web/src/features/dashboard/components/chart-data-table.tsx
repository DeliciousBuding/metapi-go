// metapi-go/features/dashboard — sr-only data summary table for charts.
//
// S10 chart-a11y alternative layer (#1035): recharts renders SVG that screen
// readers cannot walk, so each main dashboard chart also exposes its
// already-loaded series data as a visually hidden table in a simple
// "series x key points" shape. Read-only presentation layer — it never
// fetches or mutates chart data.

/** One series row: a name plus one formatted cell per key-point column. */
type ChartDataTableRow = {
  /** Series name, rendered as the row header. */
  name: string
  /** Cell values, aligned 1:1 with `columns`. */
  values: string[]
}

type ChartDataTableProps = {
  /** Table caption — announces which chart this data belongs to. */
  caption: string
  /** Header for the series-name column. */
  seriesLabel: string
  /** Key-point column headers. */
  columns: string[]
  rows: ChartDataTableRow[]
}

export function ChartDataTable({
  caption,
  seriesLabel,
  columns,
  rows,
}: ChartDataTableProps) {
  if (rows.length === 0) return null
  return (
    <table className='sr-only'>
      <caption>{caption}</caption>
      <thead>
        <tr>
          <th scope='col'>{seriesLabel}</th>
          {columns.map((column) => (
            <th key={column} scope='col'>
              {column}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={row.name}>
            <th scope='row'>{row.name}</th>
            {row.values.map((value, index) => (
              // The column header is the stable, data-derived key: cell
              // value strings may repeat (identical per-day totals).
              <td key={columns[index]}>{value}</td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

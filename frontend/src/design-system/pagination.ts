export function paginationItems(page: number, pages: number): (number | 'ellipsis-left' | 'ellipsis-right')[] {
  const last = Math.max(1, Math.floor(Number.isFinite(pages) ? pages : 1))
  const current = Math.min(last, Math.max(1, Math.floor(Number.isFinite(page) ? page : 1)))
  const numbers = [...new Set([1, Math.max(1, current - 1), current, Math.min(last, current + 1), last])].sort((a, b) => a - b)
  const result: ReturnType<typeof paginationItems> = []
  numbers.forEach((number, index) => { if (index && number - numbers[index - 1] > 1) result.push(index === 1 ? 'ellipsis-left' : 'ellipsis-right'); result.push(number) })
  return result
}

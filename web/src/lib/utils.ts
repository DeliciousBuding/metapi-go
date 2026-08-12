import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function getPageNumbers(currentPage: number, totalPages: number) {
  const maxVisiblePages = 4
  const rangeWithDots = []

  if (totalPages <= maxVisiblePages) {
    for (let i = 1; i <= totalPages; i++) {
      rangeWithDots.push(i)
    }
  } else {
    rangeWithDots.push(1)

    if (currentPage <= 2) {
      rangeWithDots.push(2)
      rangeWithDots.push('...', totalPages)
    } else if (currentPage >= totalPages - 1) {
      rangeWithDots.push('...')
      rangeWithDots.push(totalPages - 1, totalPages)
    } else {
      rangeWithDots.push('...')
      rangeWithDots.push(currentPage)
      rangeWithDots.push('...', totalPages)
    }
  }

  return rangeWithDots
}

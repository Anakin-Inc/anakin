import { act, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import type { TrackedJob } from '../types'
import { Jobs } from './Jobs'

vi.mock('../api/client', () => ({
  api: {
    getJob: vi.fn(),
    getBatchJob: vi.fn(),
  },
}))

const activeJob: TrackedJob = {
  id: 'job-1',
  url: 'https://example.com',
  type: 'single',
  status: 'processing',
  createdAt: '2026-08-10T00:00:00.000Z',
}

beforeEach(() => {
  localStorage.setItem('anakinscraper_jobs', JSON.stringify([activeJob]))
  vi.useFakeTimers()
  vi.mocked(api.getJob).mockResolvedValue({
    id: activeJob.id,
    url: activeJob.url,
    status: 'completed',
    jobType: 'url_scraper',
  })
})

afterEach(() => {
  vi.useRealTimers()
  vi.clearAllMocks()
})

describe('Jobs page', () => {
  it('polls active single jobs and persists the terminal status', async () => {
    render(
      <MemoryRouter>
        <Jobs />
      </MemoryRouter>,
    )

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000)
    })

    expect(api.getJob).toHaveBeenCalledWith('job-1')
    expect(screen.getByText('completed')).toBeInTheDocument()
    expect(JSON.parse(localStorage.getItem('anakinscraper_jobs') || '[]')[0].status).toBe('completed')
  })

  it('does not poll after unmount', async () => {
    const { unmount } = render(
      <MemoryRouter>
        <Jobs />
      </MemoryRouter>,
    )
    unmount()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000)
    })

    expect(api.getJob).not.toHaveBeenCalled()
  })
})

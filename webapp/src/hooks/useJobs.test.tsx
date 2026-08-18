import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { useJobs } from './useJobs'
import type { TrackedJob } from '../types'

const job: TrackedJob = {
  id: 'job-1',
  url: 'https://example.com',
  type: 'single',
  status: 'pending',
  createdAt: '2026-08-10T00:00:00.000Z',
}

beforeEach(() => {
  localStorage.clear()
})

describe('useJobs', () => {
  it('loads corrupt localStorage data as an empty job list', () => {
    localStorage.setItem('anakinscraper_jobs', '{not-json')

    const { result } = renderHook(() => useJobs())

    expect(result.current.jobs).toEqual([])
  })

  it('adds and persists jobs, newest first', () => {
    const { result } = renderHook(() => useJobs())

    act(() => {
      result.current.addJob(job)
    })

    expect(result.current.jobs).toEqual([job])
    expect(JSON.parse(localStorage.getItem('anakinscraper_jobs') || '[]')).toEqual([job])
  })

  it('caps persisted history at 100 jobs', () => {
    const { result } = renderHook(() => useJobs())

    act(() => {
      for (let i = 0; i < 101; i += 1) {
        result.current.addJob({ ...job, id: `job-${i}` })
      }
    })

    expect(result.current.jobs).toHaveLength(101)
    expect(JSON.parse(localStorage.getItem('anakinscraper_jobs') || '[]')).toHaveLength(100)
    expect(JSON.parse(localStorage.getItem('anakinscraper_jobs') || '[]')[0].id).toBe('job-100')
  })
})

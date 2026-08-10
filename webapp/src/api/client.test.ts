import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
})

function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('api client', () => {
  it('sends sync scrape payloads to the API proxy', async () => {
    const fetchMock = vi.fn().mockResolvedValue(response({ id: 'job-1', status: 'completed' }))
    vi.stubGlobal('fetch', fetchMock)

    await api.scrapeSync({ url: 'https://example.com', timeout: 60 })

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/scrape', {
      method: 'POST',
      body: JSON.stringify({ url: 'https://example.com', timeout: 60 }),
      headers: { 'Content-Type': 'application/json' },
    })
  })

  it('adds the configured API key header', async () => {
    const fetchMock = vi.fn().mockResolvedValue(response({ status: 'ok' }))
    vi.stubGlobal('fetch', fetchMock)
    vi.stubEnv('VITE_API_KEY', 'test-key')
    vi.resetModules()

    const { api: apiWithKey } = await import('./client')
    await apiWithKey.health()

    expect(fetchMock).toHaveBeenCalledWith('/api/health', {
      headers: {
        'Content-Type': 'application/json',
        'X-API-Key': 'test-key',
      },
    })
  })

  it('surfaces structured API errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({ message: 'URL is blocked' }, 400)))

    await expect(api.scrapeAsync({ url: 'http://127.0.0.1' })).rejects.toThrow('URL is blocked')
  })

  it('accepts empty 204 responses for deletes', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.deleteDomainConfig('example.com')).resolves.toBeUndefined()
  })
})

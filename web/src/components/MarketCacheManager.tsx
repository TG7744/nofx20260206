import { useEffect, useMemo, useState } from 'react'
import { Download, Trash2, RefreshCw, Database } from 'lucide-react'
import type { MarketCacheItem } from '../types'
import { useAuth } from '../contexts/AuthContext'

interface FilterState {
  symbol: string
  timeframe: string
  start: string
  end: string
}

const timeframes = ['1m', '3m', '5m', '15m', '30m', '1h', '2h', '4h', '1d']

export function MarketCacheManager() {
  const { token } = useAuth()
  const [filters, setFilters] = useState<FilterState>({
    symbol: '',
    timeframe: '',
    start: '',
    end: '',
  })
  const [loading, setLoading] = useState(false)
  const [items, setItems] = useState<MarketCacheItem[]>([])
  const [error, setError] = useState<string | null>(null)
  const [actionMsg, setActionMsg] = useState<string | null>(null)

  const authHeaders = useMemo<Record<string, string>>(() => {
    const h: Record<string, string> = {}
    if (token) h.Authorization = `Bearer ${token}`
    return h
  }, [token])

  const fetchList = async () => {
    setLoading(true)
    setError(null)
    setActionMsg(null)
    try {
      const params = new URLSearchParams()
      if (filters.symbol) params.set('symbol', filters.symbol.trim().toUpperCase())
      if (filters.timeframe) params.set('timeframe', filters.timeframe)
      if (filters.start) params.set('start_ts', Math.floor(new Date(filters.start).getTime()).toString())
      if (filters.end) params.set('end_ts', Math.floor(new Date(filters.end).getTime()).toString())
      const res = await fetch(`/api/market-cache?${params.toString()}`, {
        headers: {
          ...authHeaders,
        },
      })
      const data = await res.json()
      if (!res.ok) {
        throw new Error(data.error || '查询失败')
      }
      setItems(Array.isArray(data.items) ? data.items : [])
    } catch (err: any) {
      setError(err.message || '查询失败')
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async () => {
    setError(null)
    setActionMsg(null)
    if (!filters.symbol || !filters.timeframe) {
      setError('删除前请填写币种和时间周期')
      return
    }
    try {
      setLoading(true)
      const body: any = {
        symbol: filters.symbol.trim().toUpperCase(),
        timeframe: filters.timeframe,
      }
      if (filters.start) body.start_ts = Math.floor(new Date(filters.start).getTime())
      if (filters.end) body.end_ts = Math.floor(new Date(filters.end).getTime())
      const res = await fetch('/api/market-cache/delete', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...authHeaders,
        },
        body: JSON.stringify(body),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data.error || '删除失败')
      setActionMsg(data.message || '删除成功')
      await fetchList()
    } catch (err: any) {
      setError(err.message || '删除失败')
    } finally {
      setLoading(false)
    }
  }

  const handleDownload = async () => {
    setError(null)
    setActionMsg(null)
    if (!filters.symbol || !filters.timeframe) {
      setError('下载前请填写币种和时间周期')
      return
    }
    try {
      setLoading(true)
      const params = new URLSearchParams()
      params.set('symbol', filters.symbol.trim().toUpperCase())
      params.set('timeframe', filters.timeframe)
      if (filters.start) params.set('start_ts', Math.floor(new Date(filters.start).getTime()).toString())
      if (filters.end) params.set('end_ts', Math.floor(new Date(filters.end).getTime()).toString())
      const res = await fetch(`/api/market-cache/download?${params.toString()}`, {
        headers: {
          ...authHeaders,
        },
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || '下载失败')
      }
      const blob = await res.blob()
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `cache_${filters.symbol}_${filters.timeframe}.jsonl`
      document.body.appendChild(a)
      a.click()
      a.remove()
      window.URL.revokeObjectURL(url)
      setActionMsg('下载完成')
    } catch (err: any) {
      setError(err.message || '下载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div className="space-y-4">
      <div className="p-4 rounded-lg border border-[#2B3139] bg-[#0B0E11]">
        <div className="flex flex-wrap gap-3 items-end">
          <div className="flex flex-col">
            <label className="text-xs text-[#848E9C] mb-1">币种</label>
            <input
              value={filters.symbol}
              onChange={(e) => setFilters((f) => ({ ...f, symbol: e.target.value }))}
              className="px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
              placeholder="如 BTCUSDT"
            />
          </div>
          <div className="flex flex-col">
            <label className="text-xs text-[#848E9C] mb-1">时间周期</label>
            <select
              value={filters.timeframe}
              onChange={(e) => setFilters((f) => ({ ...f, timeframe: e.target.value }))}
              className="px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white min-w-[120px]"
            >
              <option value="">全部</option>
              {timeframes.map((tf) => (
                <option key={tf} value={tf}>
                  {tf}
                </option>
              ))}
            </select>
          </div>
          <div className="flex flex-col">
            <label className="text-xs text-[#848E9C] mb-1">开始时间</label>
            <input
              type="datetime-local"
              value={filters.start}
              onChange={(e) => setFilters((f) => ({ ...f, start: e.target.value }))}
              className="px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
            />
          </div>
          <div className="flex flex-col">
            <label className="text-xs text-[#848E9C] mb-1">结束时间</label>
            <input
              type="datetime-local"
              value={filters.end}
              onChange={(e) => setFilters((f) => ({ ...f, end: e.target.value }))}
              className="px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
            />
          </div>
          <button
            onClick={fetchList}
            className="flex items-center gap-2 px-3 py-2 rounded bg-[#0ECB81] text-black text-sm font-semibold hover:opacity-90"
            disabled={loading}
          >
            <RefreshCw className="w-4 h-4" />
            查询
          </button>
          <button
            onClick={handleDelete}
            className="flex items-center gap-2 px-3 py-2 rounded bg-[#F6465D] text-white text-sm font-semibold hover:opacity-90 disabled:opacity-60"
            disabled={loading}
          >
            <Trash2 className="w-4 h-4" />
            删除所选范围
          </button>
          <button
            onClick={handleDownload}
            className="flex items-center gap-2 px-3 py-2 rounded bg-[#0EA5E9] text-white text-sm font-semibold hover:opacity-90 disabled:opacity-60"
            disabled={loading}
          >
            <Download className="w-4 h-4" />
            下载所选范围
          </button>
        </div>
        {error && <div className="mt-3 text-sm text-red-400">{error}</div>}
        {actionMsg && <div className="mt-3 text-sm text-green-400">{actionMsg}</div>}
      </div>

      <div className="p-4 rounded-lg border border-[#2B3139] bg-[#0B0E11]">
        <div className="flex items-center gap-2 mb-3">
          <Database className="w-4 h-4 text-[#F0B90B]" />
          <span className="text-sm text-white">本地行情缓存</span>
          <span className="text-xs text-[#848E9C]">（按过滤条件显示，支持下载/删除）</span>
        </div>
        <div className="overflow-auto">
          <table className="min-w-full text-sm text-left text-[#EAECEF]">
            <thead className="bg-[#1E2329] text-[#848E9C]">
              <tr>
                <th className="px-3 py-2">币种</th>
                <th className="px-3 py-2">周期</th>
                <th className="px-3 py-2">开始时间</th>
                <th className="px-3 py-2">结束时间</th>
                <th className="px-3 py-2">条数</th>
                <th className="px-3 py-2">大小</th>
              </tr>
            </thead>
            <tbody>
              {items.length === 0 && (
                <tr>
                  <td className="px-3 py-3 text-center text-[#5E6673]" colSpan={6}>
                    {loading ? '加载中...' : '暂无数据'}
                  </td>
                </tr>
              )}
              {items.map((item) => (
                <tr key={`${item.symbol}-${item.timeframe}`} className="border-b border-[#2B3139]">
                  <td className="px-3 py-2">{item.symbol}</td>
                  <td className="px-3 py-2">{item.timeframe}</td>
                  <td className="px-3 py-2">{formatTs(item.start_ts)}</td>
                  <td className="px-3 py-2">{formatTs(item.end_ts)}</td>
                  <td className="px-3 py-2">{item.count}</td>
                  <td className="px-3 py-2">{formatSize(item.size_bytes)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

function formatTs(ts: number) {
  if (!ts || ts <= 0) return '-'
  const d = new Date(ts)
  return d.toISOString().replace('T', ' ').slice(0, 19)
}

function formatSize(bytes: number) {
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${bytes} B`
}

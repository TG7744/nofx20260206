import { api } from '../lib/api'
import type {
  TechBacktestResult,
  TechBacktestRun,
  TechBatchItem,
  TechBacktestRequest,
  TechBatchSummary,
  TechBatchJob,
} from '../types'
import {
  createChart,
  CrosshairMode,
  CandlestickSeries,
  HistogramSeries,
  LineSeries,
  createSeriesMarkers,
  type UTCTimestamp,
  type LineWidth,
  type SeriesMarker,
} from 'lightweight-charts'
import {
  LineChart,
  Line,
  CartesianGrid,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  Scatter,
  ComposedChart,
  Bar,
} from 'recharts'
import { useState, useEffect, useRef, useCallback } from 'react'

const timeframes = ['1m', '3m', '5m', '15m', '30m', '1h', '4h', '1d']

type IndicatorKey = 'ema' | 'vwap' | 'boll' | 'supertrend' | 'atr'

const indicatorOptions: { key: IndicatorKey; label: string }[] = [
  { key: 'ema', label: 'EMA 快/慢' },
  { key: 'vwap', label: 'VWAP' },
  { key: 'boll', label: '布林带' },
  { key: 'supertrend', label: 'Supertrend' },
  { key: 'atr', label: 'ATR 通道' },
]

function BoardChart({
  klines,
  overlay,
  signals,
  fallback,
  indicatorVisibility,
  showPriceLine,
}: {
  klines: { time: number; open: number; high: number; low: number; close: number; volume: number }[]
  overlay?: any
  signals?: { time: number; price: number; side: 'buy' | 'sell' }[]
  fallback?: React.ReactNode
  indicatorVisibility: Record<IndicatorKey, boolean>
  showPriceLine: boolean
}) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const chartRef = useRef<any>(null)
  const [failed, setFailed] = useState(false)
  const [hover, setHover] = useState<{ o: number; h: number; l: number; c: number; t: number } | null>(null)

  // 清洗指标数据，去除缺失/非数值点和明显离谱值，避免图形被拉爆
  const cleanPoints = (points: any[] | undefined, range?: { lower: number; upper: number }, allowNegative = false) =>
    (points || [])
      .filter((p) => p && Number.isFinite(p.time) && Number.isFinite(p.value))
      .filter((p) => (allowNegative ? true : p.value > 0)) // 价格类指标为正，MACD 等动量指标允许负值
      .filter((p) => {
        if (!range) return true
        return p.value >= range.lower && p.value <= range.upper
      })
      .map((p) => ({ time: p.time, value: p.value }))

  useEffect(() => {
    let dispose: (() => void) | null = null
    function loadChart() {
      if (!containerRef.current || !klines || klines.length === 0) return
      // 每次重新绘制前清除失败状态，避免一次错误后永远落到回退图表
      setFailed(false)

      // 依据价格区间过滤离谱的指标点（常见于数据缺失返回 0 或极端值）
      const closes = klines.map((k) => k.close).filter((v) => Number.isFinite(v))
      const priceRange =
        closes.length > 0
          ? {
              lower: Math.max(0, Math.min(...closes) * 0.5),
              upper: Math.max(...closes) * 2,
            }
          : undefined
      try {
        if (chartRef.current) {
          // chartRef.current 可能已在上一次清理时移除，这里做防御以避免二次 remove 抛错
          try {
            chartRef.current.remove()
          } catch {
            /* ignore double-remove */
          }
          chartRef.current = null
        }
        const chart = createChart(containerRef.current, {
          width: containerRef.current.clientWidth,
          height: 440,
          layout: {
            background: { color: '#0B0E11' },
            textColor: '#A7B4C2',
          },
          grid: {
            vertLines: { color: 'rgba(255,255,255,0.04)' },
            horzLines: { color: 'rgba(255,255,255,0.04)' },
          },
          crosshair: { mode: CrosshairMode.Normal },
          timeScale: { timeVisible: true, secondsVisible: true, borderColor: 'rgba(255,255,255,0.06)' },
          rightPriceScale: { borderColor: 'rgba(255,255,255,0.06)', scaleMargins: { top: 0.05, bottom: 0.18 } },
          localization: { dateFormat: 'MM-dd HH:mm' },
        })
        chartRef.current = chart
        const handleResize = () => {
          if (containerRef.current) chart.applyOptions({ width: containerRef.current.clientWidth })
        }
        window.addEventListener('resize', handleResize)

        const candleSeries = chart.addSeries(CandlestickSeries, {
          upColor: '#06D6A0',
          downColor: '#F65A63',
          borderUpColor: '#0ECB81',
          borderDownColor: '#F6465D',
          wickUpColor: '#0ECB81',
          wickDownColor: '#F6465D',
        })
        const priceLineColor = '#FBBF24'

        const candles = klines.map((k) => ({
          time: Math.round(k.time / 1000) as UTCTimestamp,
          open: k.open,
          high: k.high,
          low: k.low,
          close: k.close,
        }))
        candleSeries.setData(candles)

        const last = candles[candles.length - 1]
        if (showPriceLine) {
          candleSeries.createPriceLine({
            price: last.close,
            color: priceLineColor,
            lineWidth: 2,
            lineStyle: 1,
            axisLabelVisible: true,
            title: 'Last',
          })
        }

        const volSeries = chart.addSeries(HistogramSeries, {
          color: '#334155',
          priceFormat: { type: 'volume' },
          priceScaleId: 'vol',
        })
        chart.priceScale('vol').applyOptions({ scaleMargins: { top: 0.82, bottom: 0 } })
        volSeries.setData(
          klines.map((k) => ({
            time: Math.round(k.time / 1000) as UTCTimestamp,
            value: k.volume,
            color: k.close >= k.open ? 'rgba(14,203,129,0.7)' : 'rgba(246,70,93,0.7)',
          }))
        )

        // 将 MACD 曲线叠加到同一张行情图（独立价格轴，位于底部，避免与主价格区重叠）
        const macdLine = cleanPoints(overlay?.macd_line, undefined, true)
        const macdSignal = cleanPoints(overlay?.macd_signal, undefined, true)
        const macdHist = cleanPoints(overlay?.macd_hist, undefined, true)
        const hasMacd = macdLine.length > 0 && macdSignal.length > 0 && macdHist.length > 0

        if (hasMacd) {
          const macdScaleId = 'macd'
          chart.priceScale(macdScaleId).applyOptions({
            scaleMargins: { top: 0.72, bottom: 0.18 },
            borderColor: 'rgba(255,255,255,0.06)',
          })

          const macdHistSeries = chart.addSeries(HistogramSeries, {
            priceScaleId: macdScaleId,
            base: 0,
            color: '#1F2937',
            priceFormat: { type: 'price', precision: 4, minMove: 0.0001 },
          })
          macdHistSeries.setData(
            macdHist.map((p: any) => ({
              time: Math.round(p.time / 1000) as UTCTimestamp,
              value: p.value,
              color: p.value >= 0 ? 'rgba(14,203,129,0.6)' : 'rgba(246,70,93,0.6)',
            }))
          )

          const macdLineSeries = chart.addSeries(LineSeries, {
            priceScaleId: macdScaleId,
            color: '#0EA5E9',
            lineWidth: 1.4 as LineWidth,
            lastValueVisible: false,
            priceLineVisible: false,
          })
          macdLineSeries.setData(
            macdLine.map((p: any) => ({
              time: Math.round(p.time / 1000) as UTCTimestamp,
              value: p.value,
            }))
          )

          const macdSignalSeries = chart.addSeries(LineSeries, {
            priceScaleId: macdScaleId,
            color: '#F3BA2F',
            lineWidth: 1.1 as LineWidth,
            lastValueVisible: false,
            priceLineVisible: false,
          })
          macdSignalSeries.setData(
            macdSignal.map((p: any) => ({
              time: Math.round(p.time / 1000) as UTCTimestamp,
              value: p.value,
            }))
          )
        }

        const addLine = (points: any[], color: string, width = 1, dashed = false) => {
          const cleaned = cleanPoints(points, priceRange)
          if (!cleaned || cleaned.length === 0) return
          const line = chart.addSeries(LineSeries, {
            color,
            lineWidth: Math.max(1, Math.round(width)) as LineWidth,
            lineStyle: dashed ? 2 : 0,
            // 不展示指标的最新价参考线，避免右侧出现多条彩色虚线
            lastValueVisible: false,
            priceLineVisible: false,
          })
          line.setData(
            cleaned.map((p: any) => ({
              time: Math.round(p.time / 1000) as UTCTimestamp,
              value: p.value,
            }))
          )
        }

        if (indicatorVisibility.ema) {
          addLine(overlay?.ema1, '#F3BA2F', 1.4)
          addLine(overlay?.ema2, '#8B5CF6', 1.4)
        }
        if (indicatorVisibility.vwap) addLine(overlay?.vwap, '#22D3EE', 1, true)
        if (indicatorVisibility.boll) {
          addLine(overlay?.boll_upper, '#6EE7B7', 1, true)
          addLine(overlay?.boll_mid, '#38BDF8', 1.2)
          addLine(overlay?.boll_lower, '#6EE7B7', 1, true)
        }
        if (indicatorVisibility.supertrend) addLine(overlay?.supertrend, '#FB7185', 1.6)
        if (indicatorVisibility.atr) {
          addLine(overlay?.atr_upper, '#F59E0B', 0.9, true)
          addLine(overlay?.atr_lower, '#F59E0B', 0.9, true)
        }

        if (signals && signals.length > 0) {
          const markers: SeriesMarker<UTCTimestamp>[] = signals.map((s) => ({
            time: Math.round(s.time / 1000) as UTCTimestamp,
            position: s.side === 'buy' ? 'belowBar' : 'aboveBar',
            color: s.side === 'buy' ? '#0ECB81' : '#F6465D',
            shape: s.side === 'buy' ? 'arrowUp' : 'arrowDown',
            text: s.side === 'buy' ? 'B' : 'S',
          }))
          createSeriesMarkers(candleSeries, markers)
        }

        chart.subscribeCrosshairMove((param: any) => {
          if (!param || !param.time || !param.seriesData) {
            setHover(null)
            return
          }
          const data = param.seriesData.get(candleSeries)
          if (!data) {
            setHover(null)
            return
          }
          setHover({
            o: data.open,
            h: data.high,
            l: data.low,
            c: data.close,
            t: (param.time as number) * 1000,
          })
        })

        dispose = () => {
          window.removeEventListener('resize', handleResize)
          try {
            chart.remove()
          } finally {
            chartRef.current = null
          }
        }
      } catch (err) {
        console.error('lightweight-charts load failed', err)
        setFailed(true)
      }
    }
    loadChart()
    return () => {
      if (dispose) dispose()
    }
  }, [JSON.stringify(klines), JSON.stringify(overlay), JSON.stringify(signals), JSON.stringify(indicatorVisibility)])

  if (failed) {
    if (fallback) return <>{fallback}</>
    return <div className="w-full text-xs text-red-400">蜡烛图加载失败（可能被网络/CSP拦截），请检查网络或使用本地包。</div>
  }

  const last = klines[klines.length - 1]
  const first = klines[0]
  const change = last.close - first.close
  const changePct = first.close !== 0 ? (change / first.close) * 100 : 0
  const target = hover || { o: last.open, h: last.high, l: last.low, c: last.close, t: last.time }
  const priceColor = change >= 0 ? '#0ECB81' : '#F6465D'
  const fp = (v?: number) => (v === undefined || v === null || Number.isNaN(v) ? '-' : v.toFixed(2))

  return (
    <div className="w-full bg-gradient-to-b from-[#0C1017] to-[#0B0E11] border border-[#1F2933] rounded-lg shadow-inner">
      <div className="flex flex-wrap items-center justify-between px-3 py-2 text-xs text-[#A7B4C2] border-b border-[#1F2933] bg-[#0C1017]/80">
        <div className="flex items-center gap-3">
          <div className="text-sm text-white font-semibold tracking-wide">看板 · 行情图</div>
          <div className="px-2 py-[2px] rounded bg-[#111827] text-[#EAECEF] border border-[#1F2933]">{target ? fmtTime(target.t) : '-'}</div>
          <div className="flex items-center gap-2">
            <span className="text-[#6B7280]">O</span>
            <span className="text-white">{fp(target.o)}</span>
            <span className="text-[#6B7280]">H</span>
            <span className="text-white">{fp(target.h)}</span>
            <span className="text-[#6B7280]">L</span>
            <span className="text-white">{fp(target.l)}</span>
            <span className="text-[#6B7280]">C</span>
            <span className="text-white">{fp(target.c)}</span>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <div className="text-right">
            <div className="text-lg font-semibold" style={{ color: priceColor }}>
              {last.close.toFixed(2)}
            </div>
            <div className="text-[11px] text-[#A7B4C2]">
              {change >= 0 ? '+' : ''}
              {change.toFixed(2)} ({changePct >= 0 ? '+' : ''}
              {changePct.toFixed(2)}%)
            </div>
          </div>
          <div className="flex items-center gap-1 text-[11px] text-[#A7B4C2]">
            <span className="px-2 py-[3px] rounded bg-[#111827] border border-[#1F2933]">B/S</span>
          </div>
        </div>
      </div>
      <div ref={containerRef} className="w-full h-[440px]" />
    </div>
  )
}
export function TechBacktestPage() {
  const [symbol, setSymbol] = useState('BTCUSDT')
  const [timeframe, setTimeframe] = useState('5m')
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [strategyType, setStrategyType] =
    useState<'ema_cross' | 'rsi_threshold' | 'boll_breakout' | 'macd_filter' | 'cci_threshold'>('ema_cross')
  const [cciPeriod, setCciPeriod] = useState(20)
  const [cciUpper, setCciUpper] = useState(100)
  const [cciLower, setCciLower] = useState(-100)
  const [stopLoss, setStopLoss] = useState<number | ''>('')
  const [takeProfit, setTakeProfit] = useState<number | ''>('')
  const [trailStop, setTrailStop] = useState<number | ''>('')
  const [fast, setFast] = useState(20)
  const [slow, setSlow] = useState(50)
  const [rsiPeriod, setRsiPeriod] = useState(14)
  const [rsiValue, setRsiValue] = useState(30)
  const [stPeriod, setStPeriod] = useState(10)
  const [stMult, setStMult] = useState(3)
  const [initialBalance, setInitialBalance] = useState(1000)
  const [feeBps, setFeeBps] = useState(5)
  const [slippageBps, setSlippageBps] = useState(1)
  const [indicatorVisibility, setIndicatorVisibility] = useState<Record<IndicatorKey, boolean>>({
    ema: true,
    vwap: true,
    boll: true,
    supertrend: true,
    atr: true,
  })
  const [showPriceLine, setShowPriceLine] = useState(true)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<TechBacktestResult | null>(null)
  const [runId, setRunId] = useState<string | null>(null)
  const [runs, setRuns] = useState<TechBacktestRun[]>([])
  const [actionMsg, setActionMsg] = useState<string | null>(null)
  const [batchItems, setBatchItems] = useState<TechBacktestRequest[]>([])
  const [batchResults, setBatchResults] = useState<TechBatchItem[]>([])
  const [batchSummary, setBatchSummary] = useState<TechBatchSummary | null>(null)
  const [batchLoading, setBatchLoading] = useState(false)
  const [batchProgress, setBatchProgress] = useState(0)
  const [batchId, setBatchId] = useState<string | null>(null)
  const pollRef = useRef<number | null>(null)
  const [emaFastList, setEmaFastList] = useState('10,20,30')
  const [emaSlowList, setEmaSlowList] = useState('50,80,120')
  const [rsiValueList, setRsiValueList] = useState('25,30,35')
  const [rsiPeriodList, setRsiPeriodList] = useState('14')
  const [parallel, setParallel] = useState(1)
  useEffect(() => {
    return () => {
      if (pollRef.current) {
        window.clearInterval(pollRef.current)
      }
    }
  }, [])

  // poll helper
  const startPolling = (id: string) => {
    if (pollRef.current) window.clearInterval(pollRef.current)
    pollRef.current = window.setInterval(async () => {
      try {
        const job: TechBatchJob = await api.techBacktestBatchStatus(id)
        const pct = job.total > 0 ? Math.round((job.done / job.total) * 100) : 0
        setBatchProgress(pct)
        if (job.items) {
          const sorted = [...job.items].sort((a, b) => (b.result?.stats.total_return || -1e9) - (a.result?.stats.total_return || -1e9))
          setBatchResults(sorted)
        }
        setBatchSummary(job.summary || null)
        if (job.status === 'done' || job.status === 'failed') {
          if (pollRef.current) {
            window.clearInterval(pollRef.current)
            pollRef.current = null
          }
          setBatchLoading(false)
          if (job.status === 'failed') {
            setError(job.error || '批量任务失败')
          }
          setBatchId(null)
          setTimeout(() => setBatchProgress(0), 800)
        }
      } catch (err: any) {
        setError(err.message || '进度获取失败')
      }
    }, 1000)
  }

  const refreshRuns = useCallback(async () => {
    try {
      const list = await api.techBacktestRuns()
      const sorted = [...list].sort(
        (a, b) => new Date(b.started_at || '').getTime() - new Date(a.started_at || '').getTime()
      )
      setRuns(sorted)

      const targetId = runId || (sorted.length > 0 ? sorted[0].id : null)
      if (!targetId) return

      const cached = sorted.find((r) => r.id === targetId)
      if (cached?.result) {
        setResult(cached.result)
        return
      }

      try {
        const latest = await api.techBacktestResult(targetId)
        setRuns((prev) => prev.map((r) => (r.id === targetId ? latest : r)))
        if (latest.result) setResult(latest.result)
      } catch {
        // ignore fetch error for individual record
      }
    } catch (e) {
      // ignore
    }
  }, [runId])

  useEffect(() => {
    refreshRuns()
  }, [refreshRuns])

  const buildPayload = (): TechBacktestRequest => {
    let strategy: any
    if (strategyType === 'ema_cross') {
      strategy = { type: strategyType, params: { fast, slow } as Record<string, number> }
    } else if (strategyType === 'rsi_threshold') {
      strategy = { type: strategyType, params: { period: rsiPeriod, value: rsiValue, op: 1 } as Record<string, number> }
    } else if (strategyType === 'boll_breakout') {
      strategy = { type: strategyType, params: { period: slow, mult: 2 } as Record<string, number> }
    } else if (strategyType === 'cci_threshold') {
      strategy = { type: strategyType, params: { period: cciPeriod, upper: cciUpper, lower: cciLower } as Record<string, number> }
    } else {
      strategy = { type: strategyType, params: { fast, slow, signal: 9 } as Record<string, number> }
    }

    return {
      symbol: symbol.trim().toUpperCase(),
      timeframe,
      start: start ? new Date(start).toISOString() : undefined,
      end: end ? new Date(end).toISOString() : undefined,
      initial_balance: initialBalance,
      fee_bps: feeBps,
      slippage_bps: slippageBps,
      leverage: 1,
      stop_loss_pct: stopLoss === '' ? undefined : Number(stopLoss),
      take_profit_pct: takeProfit === '' ? undefined : Number(takeProfit),
      trailing_stop_pct: trailStop === '' ? undefined : Number(trailStop),
      supertrend_period: stPeriod,
      supertrend_mult: stMult,
      strategy,
    }
  }

  const run = async (asyncMode = false) => {
    setLoading(true)
    setError(null)
    try {
      const payload = buildPayload()
      if (asyncMode) {
        const res = await api.techBacktestAsync(payload)
        setRunId(res.run_id)
        setResult(null)
        setActionMsg('任务已提交，稍后自动刷新状态')
        setTimeout(refreshRuns, 1500)
      } else {
        const res = await api.techBacktest(payload)
        setResult(res)
        setRunId(null)
        await refreshRuns()
      }
    } catch (e: any) {
      setError(e.message || '回测失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="p-4 space-y-4">
      <h2 className="text-xl font-semibold text-white">技术指标回测（独立版）</h2>
      <div className="grid md:grid-cols-3 gap-3">
        <div className="p-3 bg-[#0B0E11] border border-[#2B3139] rounded space-y-2">
          <label className="block text-xs text-[#848E9C]">币种</label>
          <input
            value={symbol}
            onChange={(e) => setSymbol(e.target.value)}
            className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
          />
          <label className="block text-xs text-[#848E9C] mt-2">时间周期</label>
          <select
            value={timeframe}
            onChange={(e) => setTimeframe(e.target.value)}
            className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
          >
            {timeframes.map((tf) => (
              <option key={tf} value={tf}>
                {tf}
              </option>
            ))}
          </select>
          <label className="block text-xs text-[#848E9C] mt-2">起止时间（可选）</label>
          <input
            type="datetime-local"
            value={start}
            onChange={(e) => setStart(e.target.value)}
            className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
          />
          <input
            type="datetime-local"
            value={end}
            onChange={(e) => setEnd(e.target.value)}
            className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white mt-1"
          />
          <label className="block text-xs text-[#848E9C] mt-2">初始资金 (USDT)</label>
          <input
            type="number"
            value={initialBalance}
            onChange={(e) => setInitialBalance(Number(e.target.value))}
            className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
          />
          <div className="grid grid-cols-2 gap-2 mt-2">
            <div>
              <label className="block text-xs text-[#848E9C]">手续费 (bps)</label>
              <input
                type="number"
                value={feeBps}
                onChange={(e) => setFeeBps(Number(e.target.value))}
                className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
              />
            </div>
            <div>
              <label className="block text-xs text-[#848E9C]">滑点 (bps)</label>
              <input
                type="number"
                value={slippageBps}
                onChange={(e) => setSlippageBps(Number(e.target.value))}
                className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
              />
            </div>
          </div>
          <div className="grid grid-cols-3 gap-2 mt-2">
            <div>
              <label className="block text-xs text-[#848E9C]">止损%</label>
              <input
                type="number"
                value={stopLoss}
                onChange={(e) => setStopLoss(e.target.value === '' ? '' : Number(e.target.value))}
                className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
              />
            </div>
            <div>
              <label className="block text-xs text-[#848E9C]">止盈%</label>
              <input
                type="number"
                value={takeProfit}
                onChange={(e) => setTakeProfit(e.target.value === '' ? '' : Number(e.target.value))}
                className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
              />
            </div>
            <div>
              <label className="block text-xs text-[#848E9C]">追踪止损%</label>
              <input
                type="number"
                value={trailStop}
                onChange={(e) => setTrailStop(e.target.value === '' ? '' : Number(e.target.value))}
                className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
              />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-2 mt-2">
            <div>
              <label className="block text-xs text-[#848E9C]">Supertrend 周期</label>
              <input
                type="number"
                value={stPeriod}
                onChange={(e) => setStPeriod(Number(e.target.value))}
                className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
              />
            </div>
            <div>
              <label className="block text-xs text-[#848E9C]">Supertrend 倍数</label>
              <input
                type="number"
                value={stMult}
                onChange={(e) => setStMult(Number(e.target.value))}
                className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
                step="0.1"
              />
            </div>
          </div>
        </div>

        <div className="p-3 bg-[#0B0E11] border border-[#2B3139] rounded space-y-2">
          <label className="block text-xs text-[#848E9C]">策略类型</label>
          <select
            value={strategyType}
            onChange={(e) => setStrategyType(e.target.value as any)}
            className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
          >
            <option value="ema_cross">EMA 金叉/死叉</option>
            <option value="rsi_threshold">RSI 阈值</option>
            <option value="boll_breakout">布林下轨反转</option>
            <option value="macd_filter">MACD 金叉/死叉</option>
            <option value="cci_threshold">CCI 阈值</option>
          </select>

          {strategyType === 'ema_cross' ? (
            <div className="grid grid-cols-2 gap-2">
              <div>
                <label className="block text-xs text-[#848E9C]">快线</label>
                <input
                  type="number"
                  value={fast}
                  onChange={(e) => setFast(Number(e.target.value))}
                  className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
                />
              </div>
              <div>
                <label className="block text-xs text-[#848E9C]">慢线</label>
                <input
                  type="number"
                  value={slow}
                  onChange={(e) => setSlow(Number(e.target.value))}
                  className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
                />
              </div>
            </div>
          ) : (
            strategyType === 'rsi_threshold' ? (
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="block text-xs text-[#848E9C]">周期</label>
                  <input
                    type="number"
                    value={rsiPeriod}
                    onChange={(e) => setRsiPeriod(Number(e.target.value))}
                    className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
                  />
                </div>
                <div>
                  <label className="block text-xs text-[#848E9C]">阈值</label>
                  <input
                    type="number"
                    value={rsiValue}
                    onChange={(e) => setRsiValue(Number(e.target.value))}
                    className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
                  />
                </div>
              </div>
            ) : strategyType === 'cci_threshold' ? (
              <div className="grid grid-cols-3 gap-2">
                <div>
                  <label className="block text-xs text-[#848E9C]">周期</label>
                  <input
                    type="number"
                    value={cciPeriod}
                    onChange={(e) => setCciPeriod(Number(e.target.value))}
                    className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
                  />
                </div>
                <div>
                  <label className="block text-xs text-[#848E9C]">上限</label>
                  <input
                    type="number"
                    value={cciUpper}
                    onChange={(e) => setCciUpper(Number(e.target.value))}
                    className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
                  />
                </div>
                <div>
                  <label className="block text-xs text-[#848E9C]">下限</label>
                  <input
                    type="number"
                    value={cciLower}
                    onChange={(e) => setCciLower(Number(e.target.value))}
                    className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
                  />
                </div>
              </div>
            ) : strategyType === 'boll_breakout' ? (
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="block text-xs text-[#848E9C]">周期</label>
                  <input
                    type="number"
                    value={slow}
                    onChange={(e) => setSlow(Number(e.target.value))}
                    className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
                  />
                </div>
                <div>
                  <label className="block text-xs text-[#848E9C]">倍数</label>
                  <input
                    type="number"
                    value={fast}
                    onChange={(e) => setFast(Number(e.target.value))}
                    className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
                  />
                </div>
              </div>
            ) : (
              <div className="grid grid-cols-3 gap-2">
                <div>
                  <label className="block text-xs text-[#848E9C]">快线</label>
                  <input
                    type="number"
                    value={fast}
                    onChange={(e) => setFast(Number(e.target.value))}
                    className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
                  />
                </div>
                <div>
                  <label className="block text-xs text-[#848E9C]">慢线</label>
                  <input
                    type="number"
                    value={slow}
                    onChange={(e) => setSlow(Number(e.target.value))}
                    className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
                  />
                </div>
                <div>
                  <label className="block text-xs text-[#848E9C]">Signal</label>
                  <input
                    type="number"
                    value={rsiPeriod}
                    onChange={(e) => setRsiPeriod(Number(e.target.value))}
                    className="w-full px-3 py-2 rounded bg-[#1E2329] border border-[#2B3139] text-sm text-white"
                  />
                </div>
              </div>
            )
          )}

          <button
            onClick={() => run(false)}
            disabled={loading}
            className="w-full mt-3 py-2 rounded bg-[#0ECB81] text-black font-semibold hover:opacity-90 disabled:opacity-50"
          >
            {loading ? '回测中...' : '开始回测'}
          </button>
          <button
            onClick={() => run(true)}
            disabled={loading}
            className="w-full mt-2 py-2 rounded bg-[#0EA5E9] text-white font-semibold hover:opacity-90 disabled:opacity-50"
          >
            {loading ? '提交中...' : '异步提交'}
          </button>
          {error && <div className="text-red-400 text-sm mt-2">{error}</div>}
          {actionMsg && <div className="text-green-400 text-sm mt-1">{actionMsg}</div>}
        </div>

        <div className="p-3 bg-[#0B0E11] border border-[#2B3139] rounded space-y-2">
          <h3 className="text-sm text-white">结果概览</h3>
          {result ? (
            <>
              <div className="text-[#EAECEF] text-sm">收益: {result.stats.total_return.toFixed(2)}%</div>
              <div className="text-[#EAECEF] text-sm">最大回撤: {result.stats.max_drawdown.toFixed(2)}%</div>
              <div className="text-[#EAECEF] text-sm">交易次数: {result.stats.trades}</div>
              <div className="text-[#EAECEF] text-sm">胜率: {result.stats.win_rate.toFixed(1)}%</div>
              <div className="text-[#EAECEF] text-sm">Profit Factor: {result.stats.profit_factor.toFixed(2)}</div>
            </>
          ) : (
            <div className="text-[#5E6673] text-sm">尚无结果</div>
          )}
        </div>
      </div>

      {result && (
        <>
          {result.klines && result.klines.length > 0 && (
            <div className="p-3 bg-[#0B0E11] border border-[#2B3139] rounded">
              <div className="flex items-center justify-between">
                <h3 className="text-sm text-white">价格K线 & 信号</h3>
                <div className="text-xs text-[#848E9C]">绿▲ 买入 / 红▲ 卖出</div>
              </div>
              <div className="flex flex-wrap items-center gap-2 mt-2 text-[11px] text-[#EAECEF]">
                <span className="text-[#848E9C]">显示指标:</span>
                {indicatorOptions.map((opt) => (
                  <label key={opt.key} className="flex items-center gap-1 px-2 py-[2px] bg-[#111827] border border-[#1F2933] rounded hover:border-[#2B3139] cursor-pointer">
                    <input
                      type="checkbox"
                      className="accent-[#0ECB81] bg-[#0B0E11] border-[#2B3139]"
                      checked={indicatorVisibility[opt.key]}
                      onChange={(e) =>
                        setIndicatorVisibility((prev) => ({
                          ...prev,
                          [opt.key]: e.target.checked,
                        }))
                      }
                    />
                    <span>{opt.label}</span>
                  </label>
                ))}
                <label className="flex items-center gap-1 px-2 py-[2px] bg-[#111827] border border-[#1F2933] rounded hover:border-[#2B3139] cursor-pointer">
                  <input
                    type="checkbox"
                    className="accent-[#FBBF24] bg-[#0B0E11] border-[#2B3139]"
                    checked={showPriceLine}
                    onChange={(e) => setShowPriceLine(e.target.checked)}
                  />
                  <span>Last 价格线</span>
                </label>
              </div>
              <div className="mt-2">
                <BoardChart
                  klines={result.klines}
                  overlay={result.overlay}
                  signals={result.signals}
                  indicatorVisibility={indicatorVisibility}
                  showPriceLine={showPriceLine}
                  fallback={
                    <ResponsiveContainer width="100%" height={360}>
                      <ComposedChart
                        data={result.klines.map((k, idx) => ({
                          t: fmtTime(k.time),
                          close: k.close,
                          volume: k.volume,
                          macd_line: result.overlay?.macd_line?.[idx]?.value ?? null,
                          macd_signal: result.overlay?.macd_signal?.[idx]?.value ?? null,
                          macd_hist: result.overlay?.macd_hist?.[idx]?.value ?? null,
                        }))}
                      >
                        <CartesianGrid stroke="#2B3139" />
                        <XAxis dataKey="t" tick={{ fontSize: 10, fill: '#848E9C' }} hide />
                        <YAxis yAxisId="price" tick={{ fontSize: 10, fill: '#848E9C' }} stroke="#2B3139" domain={['auto', 'auto']} />
                        <YAxis
                          yAxisId="vol"
                          orientation="right"
                          tick={{ fontSize: 10, fill: '#5E6673' }}
                          stroke="#2B3139"
                          domain={[0, (dataMax: number) => dataMax * 1.2]}
                        />
                        <YAxis yAxisId="macd" hide domain={['auto', 'auto']} />
                        <Tooltip
                          contentStyle={{ background: '#0B0E11', border: '1px solid #2B3139', color: '#EAECEF' }}
                          labelStyle={{ color: '#EAECEF' }}
                        />
                        <Line yAxisId="price" type="monotone" dataKey="close" stroke="#0EA5E9" dot={false} strokeWidth={1.1} />
                        <Bar yAxisId="vol" dataKey="volume" fill="#3A4250" barSize={2} radius={[2, 2, 0, 0]} opacity={0.7} />
                        <Bar
                          yAxisId="macd"
                          dataKey="macd_hist"
                          barSize={3}
                          radius={[1, 1, 0, 0]}
                          shape={(props: any) => {
                            const { x, y, width, height, payload } = props
                            if (x === undefined || y === undefined || width === undefined || height === undefined) return null
                            const color = (payload?.macd_hist ?? 0) >= 0 ? '#16A34A' : '#F43F5E'
                            return <rect x={x} y={y} width={width} height={height} fill={color} rx={1} ry={1} />
                          }}
                        />
                        <Line yAxisId="macd" type="monotone" dataKey="macd_line" stroke="#0EA5E9" dot={false} strokeWidth={1} />
                        <Line yAxisId="macd" type="monotone" dataKey="macd_signal" stroke="#F3BA2F" dot={false} strokeWidth={0.9} />
                      </ComposedChart>
                    </ResponsiveContainer>
                  }
                />
              </div>
            </div>
          )}

          {/* RSI panel */}
          {result.overlay?.rsi && (
            <div className="p-3 bg-[#0B0E11] border border-[#2B3139] rounded">
              <h3 className="text-sm text-white mb-1">RSI</h3>
              <div className="h-32">
                <ResponsiveContainer width="100%" height="100%">
                  <ComposedChart data={result.overlay.rsi.map((p) => ({ t: fmtTime(p.time), value: p.value }))}>
                    <CartesianGrid stroke="#2B3139" />
                    <XAxis dataKey="t" tick={{ fontSize: 10, fill: '#848E9C' }} hide />
                    <YAxis tick={{ fontSize: 10, fill: '#848E9C' }} stroke="#2B3139" domain={[0, 100]} />
                    <Tooltip contentStyle={{ background: '#0B0E11', border: '1px solid #2B3139', color: '#EAECEF' }} labelStyle={{ color: '#EAECEF' }} />
                    <Line type="monotone" dataKey="value" stroke="#22D3EE" dot={false} strokeWidth={1.2} />
                    <Line type="monotone" data={[{ t: '', value: 70 }, { t: ' ', value: 70 }]} dataKey="value" stroke="#F6465D" dot={false} strokeDasharray="3 3" />
                    <Line type="monotone" data={[{ t: '', value: 30 }, { t: ' ', value: 30 }]} dataKey="value" stroke="#0ECB81" dot={false} strokeDasharray="3 3" />
                  </ComposedChart>
                </ResponsiveContainer>
              </div>
            </div>
          )}

          {/* ATR panel */}
          {result.overlay?.atr && (
            <div className="p-3 bg-[#0B0E11] border border-[#2B3139] rounded">
              <h3 className="text-sm text-white mb-1">ATR</h3>
              <div className="h-32">
                <ResponsiveContainer width="100%" height="100%">
                  <ComposedChart data={result.overlay.atr.map((p) => ({ t: fmtTime(p.time), value: p.value }))}>
                    <CartesianGrid stroke="#2B3139" />
                    <XAxis dataKey="t" tick={{ fontSize: 10, fill: '#848E9C' }} hide />
                    <YAxis tick={{ fontSize: 10, fill: '#848E9C' }} stroke="#2B3139" domain={['auto', 'auto']} />
                    <Tooltip
                      contentStyle={{ background: '#0B0E11', border: '1px solid #2B3139', color: '#EAECEF' }}
                      labelStyle={{ color: '#EAECEF' }}
                    />
                    <Line type="monotone" dataKey="value" stroke="#FB7185" dot={false} strokeWidth={1.2} />
                  </ComposedChart>
                </ResponsiveContainer>
              </div>
            </div>
          )}

          <div className="p-3 bg-[#0B0E11] border border-[#2B3139] rounded">
            <div className="flex items-center justify-between">
              <h3 className="text-sm text-white">收益曲线</h3>
              <button
                className="px-3 py-1 text-xs bg-[#1E2329] text-[#EAECEF] border border-[#2B3139] rounded hover:opacity-80"
                onClick={() => {
                  const rows = result.equity.map((p) => `${fmtTime(p.time)},${p.equity}`)
                  const csv = ['time,equity', ...rows].join('\n')
                  const blob = new Blob([csv], { type: 'text/csv' })
                  const url = URL.createObjectURL(blob)
                  const a = document.createElement('a')
                  a.href = url
                  a.download = 'equity.csv'
                  a.click()
                  URL.revokeObjectURL(url)
                }}
              >
                导出曲线
              </button>
            </div>
            <div className="h-64 mt-2">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart
                  data={result.equity.map((p) => ({
                    t: fmtTime(p.time),
                    equity: p.equity,
                    time: p.time,
                  }))}
                >
                  <CartesianGrid stroke="#2B3139" />
                  <XAxis dataKey="t" tick={{ fontSize: 10, fill: '#848E9C' }} hide />
                  <YAxis tick={{ fontSize: 10, fill: '#848E9C' }} stroke="#2B3139" domain={['dataMin', 'dataMax']} />
                  <Tooltip
                    contentStyle={{ background: '#0B0E11', border: '1px solid #2B3139', color: '#EAECEF' }}
                    labelStyle={{ color: '#EAECEF' }}
                  />
                  <Line type="monotone" dataKey="equity" stroke="#0ECB81" dot={false} strokeWidth={2} />
                  {result.signals && result.signals.length > 0 && (
                    <>
                      <Scatter
                        data={result.signals.filter((s) => s.side === 'buy').map((s) => ({ t: fmtTime(s.time), price: s.price }))}
                        name="Buy"
                        fill="#0ECB81"
                        shape="triangle"
                        line={false}
                      />
                      <Scatter
                        data={result.signals.filter((s) => s.side === 'sell').map((s) => ({ t: fmtTime(s.time), price: s.price }))}
                        name="Sell"
                        fill="#F6465D"
                        shape="triangle"
                        line={false}
                      />
                    </>
                  )}
                </LineChart>
              </ResponsiveContainer>
            </div>
          </div>

          <div className="p-3 bg-[#0B0E11] border border-[#2B3139] rounded">
            <h3 className="text-sm text-white mb-2">交易明细</h3>
            {result.signals && result.signals.length > 0 && (
              <div className="text-xs text-[#848E9C] mb-2">
                标记：绿色三角=买入，红色倒三角=卖出
              </div>
            )}
            <div className="overflow-auto max-h-[360px]">
              <table className="min-w-full text-sm text-left text-[#EAECEF]">
                <thead className="bg-[#1E2329] text-[#848E9C]">
                  <tr>
                    <th className="px-2 py-1">入场</th>
                    <th className="px-2 py-1">出场</th>
                    <th className="px-2 py-1">开仓价</th>
                    <th className="px-2 py-1">平仓价</th>
                    <th className="px-2 py-1">数量</th>
                    <th className="px-2 py-1">PnL</th>
                    <th className="px-2 py-1">PnL%</th>
                  </tr>
                </thead>
                <tbody>
                  {result.trades.length === 0 && (
                    <tr>
                      <td colSpan={7} className="px-2 py-2 text-center text-[#5E6673]">
                        无交易
                      </td>
                    </tr>
                  )}
                  {result.trades.map((t, idx) => (
                    <tr key={idx} className="border-b border-[#2B3139]">
                      <td className="px-2 py-1">{fmtTime(t.entry_time)}</td>
                      <td className="px-2 py-1">{fmtTime(t.exit_time)}</td>
                      <td className="px-2 py-1">{t.entry_px.toFixed(4)}</td>
                      <td className="px-2 py-1">{t.exit_px.toFixed(4)}</td>
                      <td className="px-2 py-1">{t.qty.toFixed(4)}</td>
                      <td className="px-2 py-1">{t.pnl.toFixed(4)}</td>
                      <td className={`px-2 py-1 ${t.pnl >= 0 ? 'text-green-400' : 'text-red-400'}`}>
                        {t.pnl_pct.toFixed(2)}%
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </>
      )}

      <div className="p-3 bg-[#0B0E11] border border-[#2B3139] rounded">
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-sm text-white">批量/网格回测</h3>
          <div className="flex gap-2">
            <button
              className="px-3 py-1 text-xs bg-[#0ECB81] text-black rounded hover:opacity-90"
              onClick={() => setBatchItems((prev) => [...prev, buildPayload()])}
            >
              添加当前配置
            </button>
            <button
              className="px-3 py-1 text-xs bg-[#1E2329] text-[#EAECEF] border border-[#2B3139] rounded hover:opacity-80"
              disabled={batchItems.length === 0 || batchLoading || !!batchId}
              onClick={async () => {
                setBatchLoading(true)
                setError(null)
                try {
                  setBatchResults([])
                  setBatchSummary(null)
                  setBatchProgress(0)
                  const res = await api.techBacktestBatchAsync({ items: batchItems, parallel })
                  setBatchId(res.batch_id)
                  startPolling(res.batch_id)
                } catch (e: any) {
                  setError(e.message || '批量回测失败')
                  setBatchLoading(false)
                  setBatchProgress(0)
                  if (pollRef.current) {
                    window.clearInterval(pollRef.current)
                    pollRef.current = null
                  }
                } finally {
                  // loading结束在轮询里由状态触发
                }
              }}
            >
              {batchLoading ? '运行中...' : '批量运行'}
            </button>
            <button
              className="px-3 py-1 text-xs bg-[#1E2329] text-[#EAECEF] border border-[#2B3139] rounded hover:opacity-80"
              onClick={() => {
                setBatchItems([])
                setBatchResults([])
              }}
            >
              清空
            </button>
            <button
              className="px-3 py-1 text-xs bg-[#1E2329] text-[#EAECEF] border border-[#2B3139] rounded hover:opacity-80"
              disabled={batchResults.length === 0}
              onClick={() => {
                const header = 'symbol,timeframe,strategy,total_return,max_drawdown,win_rate,profit_factor,duration_ms,error'
                const rows = batchResults.map((r) =>
                  [
                    r.config.symbol,
                    r.config.timeframe,
                    r.config.strategy.type,
                    r.result?.stats.total_return ?? '',
                    r.result?.stats.max_drawdown ?? '',
                    r.result?.stats.win_rate ?? '',
                    r.result?.stats.profit_factor ?? '',
                    r.duration_ms,
                    r.error ?? '',
                  ].join(',')
                )
                const csv = [header, ...rows].join('\n')
                const blob = new Blob([csv], { type: 'text/csv' })
                const url = URL.createObjectURL(blob)
                const a = document.createElement('a')
                a.href = url
                a.download = 'batch_results.csv'
                a.click()
                URL.revokeObjectURL(url)
              }}
            >
              导出结果
            </button>
          </div>
        </div>
        <div className="mb-2 flex items-center gap-2 text-xs text-[#848E9C]">
          <span>并发 worker:</span>
          <input
            type="number"
            min={1}
            max={8}
            value={parallel}
            onChange={(e) => setParallel(Math.max(1, Number(e.target.value)))}
            className="w-16 px-2 py-1 rounded bg-[#1E2329] border border-[#2B3139] text-xs text-white"
          />
          <span className="text-[#5E6673]">（建议 1-4，避免数据源封禁）</span>
        </div>
        {batchLoading || batchProgress > 0 ? (
          <div className="mb-2">
            <div className="flex justify-between text-[11px] text-[#848E9C] mb-1">
              <span>批量进度 {batchProgress}%</span>
              {batchId && <span className="text-[#EAECEF]">ID: {batchId}</span>}
            </div>
            <div className="w-full h-2 bg-[#1E2329] rounded overflow-hidden">
              <div
                className="h-2 bg-gradient-to-r from-[#0ECB81] via-[#0EA5E9] to-[#0ECB81] transition-all duration-200"
                style={{ width: `${Math.min(batchProgress, 100)}%` }}
              />
            </div>
          </div>
        ) : null}
        <div className="grid md:grid-cols-2 gap-2 mb-2">
          <div className="p-2 border border-[#2B3139] rounded bg-[#0F1318]">
            <div className="flex items-center justify-between mb-1">
              <span className="text-xs text-[#848E9C]">EMA 网格 (fast × slow)</span>
              <button
                className="px-2 py-1 text-[11px] bg-[#0ECB81] text-black rounded hover:opacity-90"
                onClick={() => {
                  const fasts = parseNums(emaFastList)
                  const slows = parseNums(emaSlowList)
                  if (!fasts.length || !slows.length) {
                    setError('请输入有效的 fast/slow 列表，如 10,20,30')
                    return
                  }
                  const combos: TechBacktestRequest[] = []
                  fasts.forEach((f) => {
                    slows.forEach((s) => {
                      if (f >= s) return
                      combos.push({
                        ...buildPayload(),
                        strategy: { type: 'ema_cross', params: { fast: f, slow: s } },
                      })
                    })
                  })
                  setBatchItems((prev) => [...prev, ...combos])
                  setError(null)
                }}
              >
                生成组合
              </button>
            </div>
            <div className="grid grid-cols-2 gap-2">
              <input
                value={emaFastList}
                onChange={(e) => setEmaFastList(e.target.value)}
                className="w-full px-2 py-1 rounded bg-[#1E2329] border border-[#2B3139] text-xs text-white"
                placeholder="快线: 10,20,30"
              />
              <input
                value={emaSlowList}
                onChange={(e) => setEmaSlowList(e.target.value)}
                className="w-full px-2 py-1 rounded bg-[#1E2329] border border-[#2B3139] text-xs text-white"
                placeholder="慢线: 50,80,120"
              />
            </div>
          </div>
          <div className="p-2 border border-[#2B3139] rounded bg-[#0F1318]">
            <div className="flex items-center justify-between mb-1">
              <span className="text-xs text-[#848E9C]">RSI 网格 (period × value)</span>
              <button
                className="px-2 py-1 text-[11px] bg-[#0EA5E9] text-white rounded hover:opacity-90"
                onClick={() => {
                  const periods = parseNums(rsiPeriodList)
                  const values = parseNums(rsiValueList)
                  if (!periods.length || !values.length) {
                    setError('请输入有效的 RSI 列表，如 14 或 20,25')
                    return
                  }
                  const combos: TechBacktestRequest[] = []
                  periods.forEach((p) => {
                    values.forEach((v) => {
                      combos.push({
                        ...buildPayload(),
                        strategy: { type: 'rsi_threshold', params: { period: p, value: v, op: 1 } },
                      })
                    })
                  })
                  setBatchItems((prev) => [...prev, ...combos])
                  setError(null)
                }}
              >
                生成组合
              </button>
            </div>
            <div className="grid grid-cols-2 gap-2">
              <input
                value={rsiPeriodList}
                onChange={(e) => setRsiPeriodList(e.target.value)}
                className="w-full px-2 py-1 rounded bg-[#1E2329] border border-[#2B3139] text-xs text-white"
                placeholder="周期: 14"
              />
              <input
                value={rsiValueList}
                onChange={(e) => setRsiValueList(e.target.value)}
                className="w-full px-2 py-1 rounded bg-[#1E2329] border border-[#2B3139] text-xs text-white"
                placeholder="阈值: 25,30,35"
              />
            </div>
          </div>
        </div>
        <div className="overflow-auto max-h-[240px] mb-2">
          <table className="min-w-full text-sm text-left text-[#EAECEF]">
            <thead className="bg-[#1E2329] text-[#848E9C]">
              <tr>
                <th className="px-2 py-1">#</th>
                <th className="px-2 py-1">符号/周期</th>
                <th className="px-2 py-1">策略参数</th>
                <th className="px-2 py-1">止损/止盈/TSL</th>
                <th className="px-2 py-1">操作</th>
              </tr>
            </thead>
            <tbody>
              {batchItems.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-2 py-2 text-center text-[#5E6673]">
                    点击“添加当前配置”将多个组合加入列表
                  </td>
                </tr>
              )}
              {batchItems.map((item, idx) => (
                <tr key={idx} className="border-b border-[#2B3139]">
                  <td className="px-2 py-1">{idx + 1}</td>
                  <td className="px-2 py-1">
                    {item.symbol} / {item.timeframe}
                  </td>
                  <td className="px-2 py-1">
                    {item.strategy.type}{' '}
                    {item.strategy.params
                      ? Object.entries(item.strategy.params)
                          .map(([k, v]) => `${k}:${v}`)
                          .join(', ')
                      : ''}
                  </td>
                  <td className="px-2 py-1">
                    {(item.stop_loss_pct ?? '-')}/{(item.take_profit_pct ?? '-')}/{(item.trailing_stop_pct ?? '-')}
                  </td>
                  <td className="px-2 py-1">
                    <button
                      className="px-2 py-1 text-xs bg-[#1E2329] text-[#EAECEF] border border-[#2B3139] rounded hover:opacity-80"
                      onClick={() => setBatchItems((prev) => prev.filter((_, i) => i !== idx))}
                    >
                      删除
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {batchResults.length > 0 && (
          <div className="mt-3">
            {batchSummary && (
              <div className="mb-2 grid grid-cols-3 md:grid-cols-6 gap-2 text-xs text-[#EAECEF]">
                <span>数量: {batchSummary.count}（成功 {batchSummary.success} / 失败 {batchSummary.failure}）</span>
                <span>平均收益: {batchSummary.avg_return.toFixed(2)}%</span>
                <span>平均回撤: {batchSummary.avg_drawdown.toFixed(2)}%</span>
                <span>平均胜率: {batchSummary.avg_win_rate.toFixed(1)}%</span>
                <span>平均PF: {batchSummary.avg_profit_factor.toFixed(2)}</span>
                <span>总耗时: {batchSummary.total_runtime_ms} ms</span>
              </div>
            )}
            <h4 className="text-xs text-[#848E9C] mb-1">结果对比（按收益排序）</h4>
            <div className="overflow-auto max-h-[260px]">
              <table className="min-w-full text-sm text-left text-[#EAECEF]">
                <thead className="bg-[#1E2329] text-[#848E9C]">
                  <tr>
                    <th className="px-2 py-1">排名</th>
                    <th className="px-2 py-1">符号/周期</th>
                    <th className="px-2 py-1">策略</th>
                    <th className="px-2 py-1">收益%</th>
                    <th className="px-2 py-1">回撤%</th>
                    <th className="px-2 py-1">胜率%</th>
                    <th className="px-2 py-1">PF</th>
                    <th className="px-2 py-1">耗时(ms)</th>
                    <th className="px-2 py-1">状态</th>
                  </tr>
                </thead>
                <tbody>
                  {batchResults.map((item, idx) => (
                    <tr key={idx} className="border-b border-[#2B3139]">
                      <td className="px-2 py-1">{idx + 1}</td>
                      <td className="px-2 py-1">
                        {item.config.symbol}/{item.config.timeframe}
                      </td>
                      <td className="px-2 py-1">
                        {item.config.strategy.type}
                      </td>
                      {item.result ? (
                        <>
                          <td className={`px-2 py-1 ${item.result.stats.total_return >= 0 ? 'text-green-400' : 'text-red-400'}`}>
                            {item.result.stats.total_return.toFixed(2)}
                          </td>
                          <td className="px-2 py-1">{item.result.stats.max_drawdown.toFixed(2)}</td>
                          <td className="px-2 py-1">{item.result.stats.win_rate.toFixed(1)}</td>
                          <td className="px-2 py-1">{item.result.stats.profit_factor.toFixed(2)}</td>
                        </>
                      ) : (
                        <>
                          <td className="px-2 py-1 text-[#5E6673]">-</td>
                          <td className="px-2 py-1 text-[#5E6673]">-</td>
                          <td className="px-2 py-1 text-[#5E6673]">-</td>
                          <td className="px-2 py-1 text-[#5E6673]">-</td>
                        </>
                      )}
                      <td className="px-2 py-1">{item.duration_ms}</td>
                      <td className={`px-2 py-1 ${item.error ? 'text-red-400' : 'text-green-400'}`}>
                        {item.error ? item.error : 'ok'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>

      <div className="p-3 bg-[#0B0E11] border border-[#2B3139] rounded">
        <div className="flex items-center justify-between">
          <h3 className="text-sm text-white">历史任务</h3>
          <button
            onClick={refreshRuns}
            className="px-3 py-1 rounded bg-[#1E2329] text-[#EAECEF] border border-[#2B3139] text-xs hover:opacity-80"
          >
            刷新
          </button>
        </div>
        <div className="overflow-auto max-h-[220px] mt-2">
          <table className="min-w-full text-sm text-left text-[#EAECEF]">
            <thead className="bg-[#1E2329] text-[#848E9C]">
              <tr>
                <th className="px-2 py-1">ID</th>
                <th className="px-2 py-1">状态</th>
                <th className="px-2 py-1">开始</th>
                <th className="px-2 py-1">结束</th>
                <th className="px-2 py-1">操作</th>
              </tr>
            </thead>
            <tbody>
              {runs.length === 0 && (
                <tr>
                  <td className="px-2 py-2 text-center text-[#5E6673]" colSpan={5}>
                    暂无任务
                  </td>
                </tr>
              )}
              {runs.map((r) => (
                <tr key={r.id} className="border-b border-[#2B3139]">
                  <td className="px-2 py-1">{r.id}</td>
                  <td className="px-2 py-1">{r.status}</td>
                  <td className="px-2 py-1">{fmtTimeStr(r.started_at)}</td>
                  <td className="px-2 py-1">{r.ended_at ? fmtTimeStr(r.ended_at) : '-'}</td>
                  <td className="px-2 py-1">
                    <button
                      className="px-2 py-1 text-xs bg-[#0ECB81] text-black rounded hover:opacity-80"
                      onClick={async () => {
                        try {
                          setRunId(r.id)
                          const latest = await api.techBacktestResult(r.id)
                          setRuns((prev) => prev.map((p) => (p.id === r.id ? latest : p)))
                          if (latest.result) setResult(latest.result)
                        } catch (e: any) {
                          setError(e.message || '获取结果失败')
                        }
                      }}
                    >
                      查看
                    </button>
                    <button
                      className="px-2 py-1 text-xs bg-[#1E2329] text-[#EAECEF] border border-[#2B3139] rounded hover:opacity-80 ml-1"
                      onClick={() => {
                        window.open(`/api/tech-backtest/export/${r.id}`, '_blank')
                      }}
                    >
                      导出CSV
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

function fmtTime(ts: number) {
  if (!ts) return '-'
  return new Date(ts).toISOString().replace('T', ' ').slice(0, 16)
}

function fmtTimeStr(s?: string) {
  if (!s) return '-'
  return new Date(s).toISOString().replace('T', ' ').slice(0, 16)
}

function parseNums(s: string): number[] {
  return s
    .split(',')
    .map((x) => Number(x.trim()))
    .filter((x) => !isNaN(x) && x > 0)
}

import crypto from "node:crypto";
import path from "node:path";
import { readJSON, transact } from "./storage.mjs";

export class Budget {
  constructor(launchFile, sessionRoot) {
    this.file = launchFile;
    this.pricingFile = path.join(sessionRoot, "private", "pricing.json");
  }
  status() {
    const value = readJSON(this.file);
    const reservedUSD = Object.values(value.reservations).reduce((sum, entry) => sum + entry.amount, 0);
    return { ...value, estimated: value.budgetMode === "estimate", reservedUSD, remainingUSD: value.budgetMode === "estimate" ? null : Math.max(0, value.budgetUSD - value.spentUSD - reservedUSD) };
  }
  pricing(model) {
    const key = `${model.provider}/${model.id}`;
    const configured = readJSON(this.pricingFile, {})[key];
    const prices = configured ?? model.cost;
    if (!prices || !["input", "output", "cacheRead", "cacheWrite"].every(key => Number.isFinite(prices[key]) && prices[key] >= 0)) {
      throw new Error(`Unknown pricing for ${key}; set /pentest-pricing INPUT OUTPUT CACHE_READ CACHE_WRITE (USD per million tokens)`);
    }
    if (!configured && Object.values(prices).every(value => value === 0)) {
      throw new Error(`Zero/unknown pricing for ${key}; explicitly set /pentest-pricing (including four zeros only for a known free model)`);
    }
    return prices;
  }
  setPricing(model, prices) {
    if (prices.length !== 4 || prices.some(value => !Number.isFinite(value) || value < 0)) throw new Error("Supply four nonnegative USD-per-million-token prices");
    transact(this.pricingFile, {}, state => {
      state[`${model.provider}/${model.id}`] = Object.fromEntries(["input", "output", "cacheRead", "cacheWrite"].map((key, i) => [key, prices[i]]));
    });
  }
  reserve(model, payload) {
    const estimateMode = readJSON(this.file).budgetMode === "estimate";
    let prices, pricingUnknown = false;
    try { prices = this.pricing(model); }
    catch (error) {
      if (!estimateMode) throw error;
      prices = { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }; pricingUnknown = true;
    }
    const outputTokens = payload?.max_tokens ?? payload?.max_output_tokens ?? payload?.max_completion_tokens ?? payload?.generationConfig?.maxOutputTokens ?? model.maxTokens;
    if (!(model.contextWindow > 0) || !(outputTokens > 0)) throw new Error("Model context/output bounds are required for budget reservation");
    // Reserve conservatively using the entire context window, including the
    // most expensive cache rate, and the provider's requested output maximum.
    const amount = (model.contextWindow * Math.max(prices.input, prices.cacheRead, prices.cacheWrite) + outputTokens * prices.output) / 1e6;
    const id = crypto.randomUUID();
    transact(this.file, undefined, state => {
      const reserved = Object.values(state.reservations).reduce((sum, entry) => sum + entry.amount, 0);
      if (state.budgetMode !== "estimate" && state.spentUSD + reserved + amount > state.budgetUSD + 1e-9) {
        throw new Error(`Launch budget cannot fund the next bounded model request ($${state.spentUSD.toFixed(4)} spent, $${reserved.toFixed(4)} reserved of $${state.budgetUSD}); checkpoint and relaunch or select a less costly model/output bound`);
      }
      state.reservations[id] = { amount, pid: process.pid, model: `${model.provider}/${model.id}`, prices, pricingUnknown, startedAt: new Date().toISOString() };
    });
    return id;
  }
  settle(id, message) {
    if (!id) return;
    return transact(this.file, undefined, state => {
      if (state.events[id]) return;
      const reservation = state.reservations[id];
      if (!reservation) throw new Error("Unknown budget reservation");
      const usage = message.usage;
      const keys = ["input", "output", "cacheRead", "cacheWrite"];
      const metered = usage && keys.every(key => Number.isFinite(usage[key]) && usage[key] >= 0) && (keys.some(key => usage[key] > 0) || !["error", "aborted"].includes(message.stopReason));
      let cost;
      if (metered) {
        const calculated = keys.reduce((sum, key) => sum + usage[key] * reservation.prices[key] / 1e6, 0);
        // Honor provider-reported billing when it exceeds token-based accounting.
        cost = Math.max(calculated, Number.isFinite(usage.cost?.total) ? usage.cost.total : 0);
      } else {
        // Missing usage is not a free request. Retain the reserved upper bound.
        cost = reservation.amount;
      }
      state.spentUSD += cost;
      state.events[id] = { model: reservation.model, costUSD: cost, usage: usage ?? null, estimated: state.budgetMode === "estimate" || !metered, pricingUnknown: Boolean(reservation.pricingUnknown), at: new Date().toISOString() };
      delete state.reservations[id];
      return state.spentUSD;
    });
  }
}

export async function scheduleRequest(sessionRoot, rate = 50, signal) {
  if (!(rate > 0 && rate <= 50)) throw new Error("Managed request rate must be greater than zero and no more than 50 req/s");
  for (;;) {
    if (signal?.aborted) throw new Error("Request aborted before dispatch");
    const wait = transact(path.join(sessionRoot, "private", "traffic.json"), { nextAt: 0 }, state => {
      const now = Date.now();
      if (now < state.nextAt) return Math.ceil(state.nextAt - now);
      state.nextAt = now + 1000 / rate;
      return 0;
    });
    if (!wait) return;
    await new Promise((resolve, reject) => {
      const abort = () => { clearTimeout(timer); reject(new Error("Request aborted before dispatch")); };
      const timer = setTimeout(() => { signal?.removeEventListener("abort", abort); resolve(); }, wait);
      signal?.addEventListener("abort", abort, { once: true });
      if (signal?.aborted) abort();
    });
  }
}

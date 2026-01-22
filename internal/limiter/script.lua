-- Token bucket script
-- KEYS[1]: bucket key
-- ARGV[1]: limit
-- ARGV[2]: window_ms
-- ARGV[3]: burst
-- ARGV[4]: now_ms
-- ARGV[5]: ttl_ms

local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
local now_ms = tonumber(ARGV[4])
local ttl_ms = tonumber(ARGV[5])

local tokens = tonumber(redis.call("HGET", KEYS[1], "tokens"))
local last = tonumber(redis.call("HGET", KEYS[1], "last_refill"))
if tokens == nil or last == nil then
  tokens = limit + burst
  last = now_ms
end

local rate_per_ms = limit / window_ms
local delta = math.max(0, now_ms - last)
local refill = delta * rate_per_ms
tokens = math.min(limit + burst, tokens + refill)
last = now_ms

local allowed = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
end

local reset_ms = 0
if tokens < 1 then
  local need = 1 - tokens
  reset_ms = now_ms + math.floor(need / rate_per_ms)
else
  reset_ms = now_ms
end

redis.call("HSET", KEYS[1], "tokens", tokens, "last_refill", last)
redis.call("PEXPIRE", KEYS[1], ttl_ms)

return { allowed, math.floor(tokens), reset_ms }

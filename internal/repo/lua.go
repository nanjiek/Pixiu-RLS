package repo

import (
	"github.com/redis/go-redis/v9"
)

var ScriptSliding = redis.NewScript(`
-- KEYS[1] = zset_key
-- ARGV[1] = now_ms
-- ARGV[2] = window_ms
-- ARGV[3] = limit

local now    = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit  = tonumber(ARGV[3])

-- 删除窗口外的请求
redis.call('ZREMRANGEBYSCORE', KEYS[1], 0, now - window)

-- 插入当前请求
redis.call('ZADD', KEYS[1], now, now)

-- 设置过期时间，避免 key 永久存在
redis.call('PEXPIRE', KEYS[1], window + 1000)

-- 统计窗口内的请求数
local cnt = redis.call('ZCARD', KEYS[1])

-- 返回结果
if cnt > limit then
  return {0, cnt}
else
  return {1, cnt}
end
`)

var ScriptToken = redis.NewScript(`
-- KEYS[1]=tokens, KEYS[2]=last_ts
-- ARGV[1]=capacity, ARGV[2]=refill_per_ms, ARGV[3]=now_ms

local cap   = tonumber(ARGV[1])
local rate  = tonumber(ARGV[2])
local now   = tonumber(ARGV[3])

local tokens = tonumber(redis.call('GET', KEYS[1]) or cap)
local last   = tonumber(redis.call('GET', KEYS[2]) or now)

-- 补充令牌
if now > last then
  local add = (now - last) * rate
  if add > 0 then 
    tokens = math.min(cap, tokens + add)
  end
end

-- 扣令牌
local ok = 0
if tokens >= 1 then 
  tokens = tokens - 1
  ok = 1
end

-- 保存状态并设置过期时间（保证 key 不会永久存在）
redis.call('SET', KEYS[1], tokens, 'PX', 60000)
redis.call('SET', KEYS[2], now,    'PX', 60000)

return {ok, tokens}
`)

var ScriptLeaky = redis.NewScript(`
-- KEYS[1]=level, KEYS[2]=last_ts
-- ARGV[1]=rate_per_ms, ARGV[2]=now_ms, ARGV[3]=max_queue

local rate = tonumber(ARGV[1])
local now  = tonumber(ARGV[2])
local maxq = tonumber(ARGV[3])

local lvl  = tonumber(redis.call('GET', KEYS[1]) or 0)
local last = tonumber(redis.call('GET', KEYS[2]) or now)

-- 漏水
if now > last then
  local leak = (now - last) * rate
  lvl = math.max(0, lvl - leak)
end

-- 加入请求
local ok = 0
if lvl < maxq then 
  lvl = lvl + 1
  ok = 1
end

-- 保存状态并设置过期时间
redis.call('SET', KEYS[1], lvl, 'PX', 60000)
redis.call('SET', KEYS[2], now, 'PX', 60000)

return {ok, lvl}
`)

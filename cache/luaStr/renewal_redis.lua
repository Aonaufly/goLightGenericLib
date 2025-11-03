--redis 续约
--cmdable.Eval(ctx, luaString, []string{"key"}, []any{value, timeoutSecond})
--传秒数
if redis.call('GET',KEYS[1]) == ARGV[1] then
    return redis.call('EXPIRE', KEYS[1], ARGV[2])
else
    return 0
end
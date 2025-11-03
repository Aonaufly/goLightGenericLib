--redis 解锁， 保证操作原子性
--cmdable.Eval(ctx, luaString, []string{"key"}, value)
if redis.call('GET',KEYS[1]) == ARGV[1] then
    return redis.call('DEL', KEYS[1])
else
    return 0
end
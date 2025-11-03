val = redis.call('GET', KEYS[1])
if val == false then
    --key不存在（以秒为单位）
    return redis.call('SET', KEYS[1], ARVG[1], 'EX', ARVG[2])
elseif val == ARVG[1] then
    --你上次加锁成功
    redis.call('EXPIRE', KEYS[1], ARGV[2])
    return 'OK'
else
    --别人拿到了锁
    return ''
end

val = redis.call('GET', KEYS[1])
if val == false then--已经不存在KEYS[1], 可以使用SET用来提高性能
    --key不存在（以秒为单位）， 设置成功返回OK
    return redis.call('SET', KEYS[1], ARVG[1], 'EX', ARVG[2])
elseif val == ARVG[1] then
    --你上次加锁成功
    redis.call('EXPIRE', KEYS[1], ARGV[2])
    return 'OK'
else
    --别人拿到了锁
    return ''
end

# goLightGenericLib

轻量通用代码库

## 一， 使用

```
go get -u github.com/Aonaufly/goLightGenericLib
```

## 二， 核心代码

### 1， local_pool.go

```
非线程安全的对象池
```

### 2，local_countdown_manager.go

```
全局的倒计时管理器（线程安全）
```

### 3，local_cache.go

```
本地缓存
```
### 4，redis_cache.go
```
redis缓存
```
### 5, redis分布式锁
#### 5.1 string
获得加锁单元类RedisDistributeLockStrResManager管理器
```
GetInstallRedisDistributeLockStrResManager() *RedisDistributeLockStrResManager 
```
package types

// 错误
// curTryIndex 重试进度
// totalTry 重试总数
// err 错误信息
type ErrHookProgress func(curTryIndex int, totalTry int, err error)

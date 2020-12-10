-- local ans, has, cursor = {}, {}, "0"
-- repeat
--     local t = redis.call("scan", cursor)
--     local list = t[2]
--     cursor = t[1]
--     for i = 1, #list do
--         local s = list[i]
--         if has[s] == nil then
--             has[s] = 1
--             ans[#ans+1] = s
--         end
--     end
-- until cursor == "0"
--
-- for i = 1, #ans do
--     redis.call('del', ans[i])
-- end


-- 使用SCAN异步查询所有空闲会议室
-- local ans, has, cursor = {}, {}, "0"
-- repeat
--     local t = redis.call("SCAN", cursor)
--     local list = t[2]
--     for i = 1, #list do
--         local s = list[i]
--         if has[s] == nil then
--             has[s] = 1
--             local value = redis.call("get", s)
--             if value == "0" then
--                 ans[#ans + 1] = s
--             end
--         end
--     end
--     cursor = t[1]
-- until cursor == "0"
-- return ans --or return ans


-- 设置key-val 和 过期时间 INPUT: key 1 , value, unixTime
-- redis.call('set', KEYS[1], ARGV[1])
-- if KEYS[2] == 1 then
--     redis.call('expireat', KEYS[1], ARGV[2])
-- end



-- eval "for i = 1, 200000 do redis.call('SET','authToken_' .. i,i) end" 0

-- local ans, has, cursor = {}, {}, "0"
-- repeat
--     local t = redis.call("SCAN", cursor)
--     local list = t[2]
--     for i = 1, #list do
--         local s = list[i]
--         if has[s] == nil then
--             has[s] = 1
--             ans[#ans + 1] = s
--         end
--     end
--     cursor = t[1]
-- until cursor == "0"
-- return #ans --or return ans

-- eval "local keys = redis.call('keys', KEYS[1]); local values = redis.call('mget',unpack(keys)); local keyValuePairs = {};for i = 1, #keys do keyValuePairs[i] = keys[i]..':'..values[i] end; return keyValuePairs;" 1 "user*",
--
-- local keys = redis.call('keys', KEYS[1]);
-- local values = redis.call('mget',unpack(keys))
-- local keyValuePairs = {};
-- for i = 1, #keys do
--     keyValuePairs[i] = keys[i]..':'..values[i]
-- end
-- return keyValuePairs

redis.call('SET', KEYS[1], "0")
redis.call('DEL', KEYS[2])


local ans, has, cursor = {}, {}, "0"
repeat
    local t = redis.call('SCAN', cursor, "MATCH", "dirty*")
    cursor = t[1]
    local list = t[2]
    for i = 1, #list do
        local key = list[i]
        if has[key] == nil then
            has[key] = 1
            local value = redis.call('GET', key)
            ans[#ans+1] = key .. "," .. value
        end
    end
until cursor == "0"
return ans

create table roomInfo (
    RoomName VARCHAR(50) comment '会议室名称',
    RoomID VARCHAR(50) PRIMARY KEY comment '会议室ID',
    RoomPlace VARCHAR(50) comment '会议室位置'
);
create table test(
	id int not null default 0 comment '用户id'
)
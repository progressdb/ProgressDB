core engine

1. does the storing, retrieval etc


features
1. stores in memory
2. when cpu, ram allows it - backups up to hdd.


architecture
1. on init - initializes the storage stores (ram & hdd)
2. then methods are ready to take init - operations sync to the ram for fast cycle
3. hdd is a cron with low overhead that calls itself - it takes in memory to hdd

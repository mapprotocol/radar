-- Apply once to an existing database before enabling hourly statistics.
ALTER TABLE `mos`
  ADD INDEX `mos_event_statistics_index` (`project_id`, `tx_timestamp`, `event_id`, `chain_id`),
  ALGORITHM=INPLACE, LOCK=NONE;

CREATE TABLE IF NOT EXISTS `block` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `chain_id` varchar(64) DEFAULT NULL,
  `number` varchar(255) DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb3;

CREATE TABLE IF NOT EXISTS `mos` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `chain_id` bigint DEFAULT NULL,
  `event_id` bigint DEFAULT NULL,
  `project_id` bigint DEFAULT NULL,
  `tx_hash` varchar(128) DEFAULT NULL,
  `contract_address` varchar(128) DEFAULT NULL,
  `topic` varchar(512) DEFAULT NULL,
  `block_number` bigint unsigned DEFAULT NULL,
  `block_hash` varchar(128) DEFAULT NULL,
  `tx_index` int unsigned DEFAULT '0',
  `log_index` int DEFAULT '-1',
  `log_data` longtext,
  `tx_timestamp` bigint DEFAULT NULL,
  `create_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `tx_hash_unique` (`tx_hash`,`log_index`,`project_id`) USING BTREE,
  KEY `mos_select_index` (`chain_id`,`project_id`,`event_id`,`id`),
  KEY `block_index` (`block_number`,`project_id`,`chain_id`,`event_id`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `event` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `project_id` bigint DEFAULT NULL,
  `chain_id` varchar(64) DEFAULT NULL,
  `address` varchar(255) DEFAULT NULL,
  `format` varchar(512) DEFAULT NULL,
  `topic` varchar(512) DEFAULT NULL,
  `block_number` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `created_at` timestamp NULL DEFAULT NULL,
  `deleted_at` timestamp NULL DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `project` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `name` varchar(255) DEFAULT NULL,
  `description` varchar(255) DEFAULT NULL,
  `created_at` timestamp NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` timestamp NULL DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `scan_block` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `chain_id` varchar(64) DEFAULT NULL,
  `number` varchar(255) DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb3;
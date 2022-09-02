CREATE TABLE item_status_log
(
    item_name  varchar not null,
    status     varchar not null,
    changed_at int     not null
);
create index item_status_log_item_name_index on item_status_log (item_name);
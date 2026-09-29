CREATE TABLE item_status_log
(
    item_name  varchar not null,
    status     varchar not null,
    changed_at int     not null
);
create index item_status_log_item_name_index on item_status_log (item_name);
create index if not exists item_status_log_changed_at_index on item_status_log (changed_at);
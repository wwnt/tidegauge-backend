CREATE EXTENSION if not exists citext;
CREATE EXTENSION if not exists hstore;

create table stations
(
    id                uuid                               not null primary key,
    identifier        varchar                            not null,
    name              varchar                            not null,
    ip_addr           varchar     default ''             not null,
    location          jsonb       default 'null'         not null,
    partner           jsonb       default 'null'         not null,
    status            varchar     default 'Disconnected' not null,
    status_changed_at timestamptz default 'epoch'        not null,
    cameras           jsonb       default 'null'         not null,
    upstream          boolean     default false          not null,
    deleted_at        timestamptz
);
create index on stations (deleted_at);

create table devices
(
    station_id       uuid                        not null references stations on delete cascade,
    name             varchar                     not null,
    specs            jsonb       default 'null'  not null,
    last_maintenance timestamptz default 'epoch' not null,
    primary key (station_id, name)
);

create table items
(
    station_id        uuid                        not null,
    name              varchar                     not null,
    type              varchar     default ''      not null,
    device_name       varchar     default ''      not null,
    status            varchar     default ''      not null,
    status_changed_at timestamptz default 'epoch' not null,
    available         hstore      default ''      not null,
    primary key (station_id, name),
    foreign key (station_id, device_name) references devices (station_id, name) on delete cascade
);

create table device_record
(
    id               uuid                      not null primary key,
    station_id       uuid                      not null references stations on delete cascade,
    device_name      varchar                   not null,
    record           text                      not null,
    created_at       timestamptz default now() not null,
    updated_at       timestamptz default now() not null,
    upstream_version integer     default 1     not null,
    version          integer     default 1     not null
);

create table upstreams
(
    id       serial  not null primary key,
    username varchar not null,
    password varchar not null,
    url      varchar not null,
    unique (username, url)
);

create table upstream_stations
(
    upstream_id integer not null references upstreams on delete cascade,
    station_id  uuid    not null references stations on delete cascade,
    primary key (upstream_id, station_id)
);

create table users
(
    username    varchar                not null primary key,
    role        smallint default 0     not null,
    email       citext   default ''    not null,
    live_camera boolean  default false not null
);

create table permissions_item_data
(
    username   varchar not null references users (username) on delete cascade,
    station_id uuid    not null,
    item_name  varchar not null,
    primary key (username, station_id, item_name)
);

create table permissions_camera_status
(
    username    varchar not null references users (username) on delete cascade,
    station_id  uuid    not null,
    camera_name varchar not null,
    primary key (username, station_id, camera_name)
);

create table item_status_log
(
    station_id uuid        not null references stations on delete cascade,
    row_id     bigint      not null,
    item_name  varchar     not null,
    status     varchar     not null,
    changed_at timestamptz not null,
    primary key (station_id, row_id)
);

create table rpi_status_log
(
    station_id uuid             not null references stations on delete cascade,
    cpu_temp   double precision not null,
    timestamp  timestamptz      not null
);

insert into users(username, role, live_camera)
VALUES ('tgm-admin', 2, true);

create table station_info_gloss_all
(
    id                integer          not null,
    name              varchar          not null,
    country           varchar          not null,
    latitude          double precision not null,
    longitude         double precision not null,
    latest_psmsl      varchar          not null,
    latest_psmsl_rlr  varchar          not null,
    latest_bodc       varchar          not null,
    latest_sonel      varchar          not null,
    latest_jasl       varchar          not null,
    latest_uhslc_fast varchar          not null,
    latest_vliz       varchar          not null
);

create table station_sea_level
(
    code  varchar          not null,
    lat   double precision not null,
    lon   double precision not null,
    level double precision not null
);
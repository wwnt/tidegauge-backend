# 数据项（item_name）说明

本文档面向**数据分析人员**。导出的数据**每个文件对应一条数据流**，文件名即 `item_name`，文件内每行只有 `timestamp(ms), value`。

下面按传感器型号列出**当前**所有的 `item_name` 及其含义。

## OTT PLS-C（压力式水文探头）

| `item_name`                              | 解释                            |
| ---------------------------------------- | ------------------------------- |
| `location1_water_level_pls_c`            | 水位（相对参考面），单位 m      |
| `location1_water_temperature`            | 水温，单位 ℃                    |
| `location1_water_conductivity`           | 电导率,单位 mS/cm               |
| `location1_water_salinity`               | 盐度，单位 PSU                  |
| `location1_water_total_dissolved_solids` | 溶解性总固体（TDS），单位 ppm。由探头根据电导率换算：`TDS[ppm] = 0.64 × electrical_conductivity[mS/cm]`，该系数按海水环境标定，特殊场景可改 |

## OTT SE200（浮子水位计）

| `item_name`                       | 解释                                         |
| --------------------------------- | -------------------------------------------- |
| `location1_water_level_shaft`     | 水位（浮子读数），单位 m               |

## VEGA VEGAPULS 61（雷达液位计）

| `item_name`                            | 解释                                   |
| -------------------------------------- | -------------------------------------- |
| `location1_radar_water_distance`       | 雷达天线到水面的距离，单位 m           |

## Vaisala HMP155（温湿度传感器）

| `item_name`                    | 解释                  |
| ------------------------------ | --------------------- |
| `location1_air_temperature`    | 气温，单位 ℃          |
| `location1_air_humidity`       | 相对湿度，单位 %RH    |

## Vaisala PTB330（数字气压计）

| `item_name`                 | 解释                  |
| --------------------------- | --------------------- |
| `location1_air_pressure`    | 大气压，单位 hPa      |

## Vaisala PWD50（能见度传感器）

| `item_name`                   | 解释                  |
| ----------------------------- | --------------------- |
| `location1_air_visibility`    | 能见度，单位 m        |

## Vaisala WMT700（超声波风速风向仪）

| `item_name`                    | 解释                    |
| ------------------------------ | ----------------------- |
| `location1_wind_speed`         | 风速，单位 m/s          |
| `location1_wind_direction`     | 风向，单位 °（度）      |

## Vaisala RG13（翻斗式雨量计）

| `item_name`     | 解释                                              |
| --------------- | ------------------------------------------------- |
| `rain_volume`   | 上报周期内累计降雨量（默认 1 分钟），单位 mm      |

## Vaisala DRD11A（雨水检测器，模拟电压输出）

| `item_name`            | 解释                                              |
| ---------------------- | ------------------------------------------------- |
| `drd11a_analog_out`    | DRD11A 模拟输出电压，单位 V；电压随降雨增大而下降 |

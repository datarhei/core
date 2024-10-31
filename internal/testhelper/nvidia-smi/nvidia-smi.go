package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"time"
)

var pmondata = `# gpu        pid  type    sm   mem   enc   dec    fb   command
# Idx          #   C/G     %     %     %     %    MB   name
    0       7372     C     2     0     2     -   136   ffmpeg         
    0      12176     C     5     2     3     7   782   ffmpeg         
    1      20035     C     8     2     4     1  1145   ffmpeg         
    1      20141     C     2     1     1     3   429   ffmpeg         
    0      29591     C     2     1     -     2   435   ffmpeg   `

var querydata = `<?xml version="1.0" ?>
<!DOCTYPE nvidia_smi_log SYSTEM "nvsmi_device_v12.dtd">
<nvidia_smi_log>
    <timestamp>Mon Jul 15 13:41:56 2024</timestamp>
    <driver_version>555.42.06</driver_version>
    <cuda_version>12.5</cuda_version>
    <attached_gpus>2</attached_gpus>
    <gpu id="00000000:01:00.0">
        <product_name>NVIDIA L4</product_name>
        <product_brand>NVIDIA</product_brand>
        <product_architecture>Ada Lovelace</product_architecture>
        <display_mode>Enabled</display_mode>
        <display_active>Disabled</display_active>
        <persistence_mode>Disabled</persistence_mode>
        <addressing_mode>None</addressing_mode>
        <mig_mode>
            <current_mig>N/A</current_mig>
            <pending_mig>N/A</pending_mig>
        </mig_mode>
        <mig_devices>
			None
        </mig_devices>
        <accounting_mode>Disabled</accounting_mode>
        <accounting_mode_buffer_size>4000</accounting_mode_buffer_size>
        <driver_model>
            <current_dm>N/A</current_dm>
            <pending_dm>N/A</pending_dm>
        </driver_model>
        <serial>1654523003308</serial>
        <uuid>GPU-c5533cd4-5a60-059e-348d-b6d7466932e4</uuid>
        <minor_number>1</minor_number>
        <vbios_version>95.04.29.00.06</vbios_version>
        <multigpu_board>No</multigpu_board>
        <board_id>0x100</board_id>
        <board_part_number>900-2G193-0000-001</board_part_number>
        <gpu_part_number>27B8-895-A1</gpu_part_number>
        <gpu_fru_part_number>N/A</gpu_fru_part_number>
        <gpu_module_id>1</gpu_module_id>
        <inforom_version>
            <img_version>G193.0200.00.01</img_version>
            <oem_object>2.1</oem_object>
            <ecc_object>6.16</ecc_object>
            <pwr_object>N/A</pwr_object>
        </inforom_version>
        <inforom_bbx_flush>
            <latest_timestamp>N/A</latest_timestamp>
            <latest_duration>N/A</latest_duration>
        </inforom_bbx_flush>
        <gpu_operation_mode>
            <current_gom>N/A</current_gom>
            <pending_gom>N/A</pending_gom>
        </gpu_operation_mode>
        <c2c_mode>N/A</c2c_mode>
        <gpu_virtualization_mode>
            <virtualization_mode>None</virtualization_mode>
            <host_vgpu_mode>N/A</host_vgpu_mode>
            <vgpu_heterogeneous_mode>N/A</vgpu_heterogeneous_mode>
        </gpu_virtualization_mode>
        <gpu_reset_status>
            <reset_required>No</reset_required>
            <drain_and_reset_recommended>N/A</drain_and_reset_recommended>
        </gpu_reset_status>
        <gsp_firmware_version>555.42.06</gsp_firmware_version>
        <ibmnpu>
            <relaxed_ordering_mode>N/A</relaxed_ordering_mode>
        </ibmnpu>
        <pci>
            <pci_bus>01</pci_bus>
            <pci_device>00</pci_device>
            <pci_domain>0000</pci_domain>
            <pci_base_class>3</pci_base_class>
            <pci_sub_class>2</pci_sub_class>
            <pci_device_id>27B810DE</pci_device_id>
            <pci_bus_id>00000000:01:00.0</pci_bus_id>
            <pci_sub_system_id>16CA10DE</pci_sub_system_id>
            <pci_gpu_link_info>
                <pcie_gen>
                    <max_link_gen>4</max_link_gen>
                    <current_link_gen>4</current_link_gen>
                    <device_current_link_gen>4</device_current_link_gen>
                    <max_device_link_gen>4</max_device_link_gen>
                    <max_host_link_gen>5</max_host_link_gen>
                </pcie_gen>
                <link_widths>
                    <max_link_width>16x</max_link_width>
                    <current_link_width>16x</current_link_width>
                </link_widths>
            </pci_gpu_link_info>
            <pci_bridge_chip>
                <bridge_chip_type>N/A</bridge_chip_type>
                <bridge_chip_fw>N/A</bridge_chip_fw>
            </pci_bridge_chip>
            <replay_counter>0</replay_counter>
            <replay_rollover_counter>0</replay_rollover_counter>
            <tx_util>0 KB/s</tx_util>
            <rx_util>0 KB/s</rx_util>
            <atomic_caps_inbound>N/A</atomic_caps_inbound>
            <atomic_caps_outbound>N/A</atomic_caps_outbound>
        </pci>
        <fan_speed>N/A</fan_speed>
        <performance_state>P0</performance_state>
        <clocks_event_reasons>
            <clocks_event_reason_gpu_idle>Active</clocks_event_reason_gpu_idle>
            <clocks_event_reason_applications_clocks_setting>Not Active</clocks_event_reason_applications_clocks_setting>
            <clocks_event_reason_sw_power_cap>Not Active</clocks_event_reason_sw_power_cap>
            <clocks_event_reason_hw_slowdown>Not Active</clocks_event_reason_hw_slowdown>
            <clocks_event_reason_hw_thermal_slowdown>Not Active</clocks_event_reason_hw_thermal_slowdown>
            <clocks_event_reason_hw_power_brake_slowdown>Not Active</clocks_event_reason_hw_power_brake_slowdown>
            <clocks_event_reason_sync_boost>Not Active</clocks_event_reason_sync_boost>
            <clocks_event_reason_sw_thermal_slowdown>Not Active</clocks_event_reason_sw_thermal_slowdown>
            <clocks_event_reason_display_clocks_setting>Not Active</clocks_event_reason_display_clocks_setting>
        </clocks_event_reasons>
        <sparse_operation_mode>N/A</sparse_operation_mode>
        <fb_memory_usage>
            <total>23034 MiB</total>
            <reserved>434 MiB</reserved>
            <used>1 MiB</used>
            <free>22601 MiB</free>
        </fb_memory_usage>
        <bar1_memory_usage>
            <total>32768 MiB</total>
            <used>1 MiB</used>
            <free>32767 MiB</free>
        </bar1_memory_usage>
        <cc_protected_memory_usage>
            <total>0 MiB</total>
            <used>0 MiB</used>
            <free>0 MiB</free>
        </cc_protected_memory_usage>
        <compute_mode>Default</compute_mode>
        <utilization>
            <gpu_util>2 %</gpu_util>
            <memory_util>0 %</memory_util>
            <encoder_util>0 %</encoder_util>
            <decoder_util>0 %</decoder_util>
            <jpeg_util>0 %</jpeg_util>
            <ofa_util>0 %</ofa_util>
        </utilization>
        <encoder_stats>
            <session_count>0</session_count>
            <average_fps>0</average_fps>
            <average_latency>0</average_latency>
        </encoder_stats>
        <fbc_stats>
            <session_count>0</session_count>
            <average_fps>0</average_fps>
            <average_latency>0</average_latency>
        </fbc_stats>
        <ecc_mode>
            <current_ecc>Enabled</current_ecc>
            <pending_ecc>Enabled</pending_ecc>
        </ecc_mode>
        <ecc_errors>
            <volatile>
                <sram_correctable>0</sram_correctable>
                <sram_uncorrectable_parity>0</sram_uncorrectable_parity>
                <sram_uncorrectable_secded>0</sram_uncorrectable_secded>
                <dram_correctable>0</dram_correctable>
                <dram_uncorrectable>0</dram_uncorrectable>
            </volatile>
            <aggregate>
                <sram_correctable>0</sram_correctable>
                <sram_uncorrectable_parity>0</sram_uncorrectable_parity>
                <sram_uncorrectable_secded>0</sram_uncorrectable_secded>
                <dram_correctable>0</dram_correctable>
                <dram_uncorrectable>0</dram_uncorrectable>
                <sram_threshold_exceeded>No</sram_threshold_exceeded>
            </aggregate>
            <aggregate_uncorrectable_sram_sources>
                <sram_l2>0</sram_l2>
                <sram_sm>0</sram_sm>
                <sram_microcontroller>0</sram_microcontroller>
                <sram_pcie>0</sram_pcie>
                <sram_other>0</sram_other>
            </aggregate_uncorrectable_sram_sources>
        </ecc_errors>
        <retired_pages>
            <multiple_single_bit_retirement>
                <retired_count>N/A</retired_count>
                <retired_pagelist>N/A</retired_pagelist>
            </multiple_single_bit_retirement>
            <double_bit_retirement>
                <retired_count>N/A</retired_count>
                <retired_pagelist>N/A</retired_pagelist>
            </double_bit_retirement>
            <pending_blacklist>N/A</pending_blacklist>
            <pending_retirement>N/A</pending_retirement>
        </retired_pages>
        <remapped_rows>
            <remapped_row_corr>0</remapped_row_corr>
            <remapped_row_unc>0</remapped_row_unc>
            <remapped_row_pending>No</remapped_row_pending>
            <remapped_row_failure>No</remapped_row_failure>
            <row_remapper_histogram>
                <row_remapper_histogram_max>96 bank(s)</row_remapper_histogram_max>
                <row_remapper_histogram_high>0 bank(s)</row_remapper_histogram_high>
                <row_remapper_histogram_partial>0 bank(s)</row_remapper_histogram_partial>
                <row_remapper_histogram_low>0 bank(s)</row_remapper_histogram_low>
                <row_remapper_histogram_none>0 bank(s)</row_remapper_histogram_none>
            </row_remapper_histogram>
        </remapped_rows>
        <temperature>
            <gpu_temp>45 C</gpu_temp>
            <gpu_temp_tlimit>39 C</gpu_temp_tlimit>
            <gpu_temp_max_tlimit_threshold>-5 C</gpu_temp_max_tlimit_threshold>
            <gpu_temp_slow_tlimit_threshold>-2 C</gpu_temp_slow_tlimit_threshold>
            <gpu_temp_max_gpu_tlimit_threshold>0 C</gpu_temp_max_gpu_tlimit_threshold>
            <gpu_target_temperature>N/A</gpu_target_temperature>
            <memory_temp>N/A</memory_temp>
            <gpu_temp_max_mem_tlimit_threshold>N/A</gpu_temp_max_mem_tlimit_threshold>
        </temperature>
        <supported_gpu_target_temp>
            <gpu_target_temp_min>N/A</gpu_target_temp_min>
            <gpu_target_temp_max>N/A</gpu_target_temp_max>
        </supported_gpu_target_temp>
        <gpu_power_readings>
            <power_state>P0</power_state>
            <power_draw>27.22 W</power_draw>
            <current_power_limit>72.00 W</current_power_limit>
            <requested_power_limit>72.00 W</requested_power_limit>
            <default_power_limit>72.00 W</default_power_limit>
            <min_power_limit>40.00 W</min_power_limit>
            <max_power_limit>72.00 W</max_power_limit>
        </gpu_power_readings>
        <gpu_memory_power_readings>
            <power_draw>N/A</power_draw>
        </gpu_memory_power_readings>
        <module_power_readings>
            <power_state>P0</power_state>
            <power_draw>N/A</power_draw>
            <current_power_limit>N/A</current_power_limit>
            <requested_power_limit>N/A</requested_power_limit>
            <default_power_limit>N/A</default_power_limit>
            <min_power_limit>N/A</min_power_limit>
            <max_power_limit>N/A</max_power_limit>
        </module_power_readings>
        <clocks>
            <graphics_clock>2040 MHz</graphics_clock>
            <sm_clock>2040 MHz</sm_clock>
            <mem_clock>6250 MHz</mem_clock>
            <video_clock>1770 MHz</video_clock>
        </clocks>
        <applications_clocks>
            <graphics_clock>2040 MHz</graphics_clock>
            <mem_clock>6251 MHz</mem_clock>
        </applications_clocks>
        <default_applications_clocks>
            <graphics_clock>2040 MHz</graphics_clock>
            <mem_clock>6251 MHz</mem_clock>
        </default_applications_clocks>
        <deferred_clocks>
            <mem_clock>N/A</mem_clock>
        </deferred_clocks>
        <max_clocks>
            <graphics_clock>2040 MHz</graphics_clock>
            <sm_clock>2040 MHz</sm_clock>
            <mem_clock>6251 MHz</mem_clock>
            <video_clock>1770 MHz</video_clock>
        </max_clocks>
        <max_customer_boost_clocks>
            <graphics_clock>2040 MHz</graphics_clock>
        </max_customer_boost_clocks>
        <clock_policy>
            <auto_boost>N/A</auto_boost>
            <auto_boost_default>N/A</auto_boost_default>
        </clock_policy>
        <voltage>
            <graphics_volt>885.000 mV</graphics_volt>
        </voltage>
        <fabric>
            <state>N/A</state>
            <status>N/A</status>
            <cliqueId>N/A</cliqueId>
            <clusterUuid>N/A</clusterUuid>
            <health>
                <bandwidth>N/A</bandwidth>
            </health>
        </fabric>
        <supported_clocks>
            <supported_mem_clock>
                <value>6251 MHz</value>
                <supported_graphics_clock>2040 MHz</supported_graphics_clock>
                <supported_graphics_clock>2025 MHz</supported_graphics_clock>
                <supported_graphics_clock>2010 MHz</supported_graphics_clock>
                <supported_graphics_clock>1995 MHz</supported_graphics_clock>
                <supported_graphics_clock>1980 MHz</supported_graphics_clock>
                <supported_graphics_clock>1965 MHz</supported_graphics_clock>
                <supported_graphics_clock>1950 MHz</supported_graphics_clock>
                <supported_graphics_clock>1935 MHz</supported_graphics_clock>
                <supported_graphics_clock>1920 MHz</supported_graphics_clock>
                <supported_graphics_clock>1905 MHz</supported_graphics_clock>
                <supported_graphics_clock>1890 MHz</supported_graphics_clock>
                <supported_graphics_clock>1875 MHz</supported_graphics_clock>
                <supported_graphics_clock>1860 MHz</supported_graphics_clock>
                <supported_graphics_clock>1845 MHz</supported_graphics_clock>
                <supported_graphics_clock>1830 MHz</supported_graphics_clock>
                <supported_graphics_clock>1815 MHz</supported_graphics_clock>
                <supported_graphics_clock>1800 MHz</supported_graphics_clock>
                <supported_graphics_clock>1785 MHz</supported_graphics_clock>
                <supported_graphics_clock>1770 MHz</supported_graphics_clock>
                <supported_graphics_clock>1755 MHz</supported_graphics_clock>
                <supported_graphics_clock>1740 MHz</supported_graphics_clock>
                <supported_graphics_clock>1725 MHz</supported_graphics_clock>
                <supported_graphics_clock>1710 MHz</supported_graphics_clock>
                <supported_graphics_clock>1695 MHz</supported_graphics_clock>
                <supported_graphics_clock>1680 MHz</supported_graphics_clock>
                <supported_graphics_clock>1665 MHz</supported_graphics_clock>
                <supported_graphics_clock>1650 MHz</supported_graphics_clock>
                <supported_graphics_clock>1635 MHz</supported_graphics_clock>
                <supported_graphics_clock>1620 MHz</supported_graphics_clock>
                <supported_graphics_clock>1605 MHz</supported_graphics_clock>
                <supported_graphics_clock>1590 MHz</supported_graphics_clock>
                <supported_graphics_clock>1575 MHz</supported_graphics_clock>
                <supported_graphics_clock>1560 MHz</supported_graphics_clock>
                <supported_graphics_clock>1545 MHz</supported_graphics_clock>
                <supported_graphics_clock>1530 MHz</supported_graphics_clock>
                <supported_graphics_clock>1515 MHz</supported_graphics_clock>
                <supported_graphics_clock>1500 MHz</supported_graphics_clock>
                <supported_graphics_clock>1485 MHz</supported_graphics_clock>
                <supported_graphics_clock>1470 MHz</supported_graphics_clock>
                <supported_graphics_clock>1455 MHz</supported_graphics_clock>
                <supported_graphics_clock>1440 MHz</supported_graphics_clock>
                <supported_graphics_clock>1425 MHz</supported_graphics_clock>
                <supported_graphics_clock>1410 MHz</supported_graphics_clock>
                <supported_graphics_clock>1395 MHz</supported_graphics_clock>
                <supported_graphics_clock>1380 MHz</supported_graphics_clock>
                <supported_graphics_clock>1365 MHz</supported_graphics_clock>
                <supported_graphics_clock>1350 MHz</supported_graphics_clock>
                <supported_graphics_clock>1335 MHz</supported_graphics_clock>
                <supported_graphics_clock>1320 MHz</supported_graphics_clock>
                <supported_graphics_clock>1305 MHz</supported_graphics_clock>
                <supported_graphics_clock>1290 MHz</supported_graphics_clock>
                <supported_graphics_clock>1275 MHz</supported_graphics_clock>
                <supported_graphics_clock>1260 MHz</supported_graphics_clock>
                <supported_graphics_clock>1245 MHz</supported_graphics_clock>
                <supported_graphics_clock>1230 MHz</supported_graphics_clock>
                <supported_graphics_clock>1215 MHz</supported_graphics_clock>
                <supported_graphics_clock>1200 MHz</supported_graphics_clock>
                <supported_graphics_clock>1185 MHz</supported_graphics_clock>
                <supported_graphics_clock>1170 MHz</supported_graphics_clock>
                <supported_graphics_clock>1155 MHz</supported_graphics_clock>
                <supported_graphics_clock>1140 MHz</supported_graphics_clock>
                <supported_graphics_clock>1125 MHz</supported_graphics_clock>
                <supported_graphics_clock>1110 MHz</supported_graphics_clock>
                <supported_graphics_clock>1095 MHz</supported_graphics_clock>
                <supported_graphics_clock>1080 MHz</supported_graphics_clock>
                <supported_graphics_clock>1065 MHz</supported_graphics_clock>
                <supported_graphics_clock>1050 MHz</supported_graphics_clock>
                <supported_graphics_clock>1035 MHz</supported_graphics_clock>
                <supported_graphics_clock>1020 MHz</supported_graphics_clock>
                <supported_graphics_clock>1005 MHz</supported_graphics_clock>
                <supported_graphics_clock>990 MHz</supported_graphics_clock>
                <supported_graphics_clock>975 MHz</supported_graphics_clock>
                <supported_graphics_clock>960 MHz</supported_graphics_clock>
                <supported_graphics_clock>945 MHz</supported_graphics_clock>
                <supported_graphics_clock>930 MHz</supported_graphics_clock>
                <supported_graphics_clock>915 MHz</supported_graphics_clock>
                <supported_graphics_clock>900 MHz</supported_graphics_clock>
                <supported_graphics_clock>885 MHz</supported_graphics_clock>
                <supported_graphics_clock>870 MHz</supported_graphics_clock>
                <supported_graphics_clock>855 MHz</supported_graphics_clock>
                <supported_graphics_clock>840 MHz</supported_graphics_clock>
                <supported_graphics_clock>825 MHz</supported_graphics_clock>
                <supported_graphics_clock>810 MHz</supported_graphics_clock>
                <supported_graphics_clock>795 MHz</supported_graphics_clock>
                <supported_graphics_clock>780 MHz</supported_graphics_clock>
                <supported_graphics_clock>765 MHz</supported_graphics_clock>
                <supported_graphics_clock>750 MHz</supported_graphics_clock>
                <supported_graphics_clock>735 MHz</supported_graphics_clock>
                <supported_graphics_clock>720 MHz</supported_graphics_clock>
                <supported_graphics_clock>705 MHz</supported_graphics_clock>
                <supported_graphics_clock>690 MHz</supported_graphics_clock>
                <supported_graphics_clock>675 MHz</supported_graphics_clock>
                <supported_graphics_clock>660 MHz</supported_graphics_clock>
                <supported_graphics_clock>645 MHz</supported_graphics_clock>
                <supported_graphics_clock>630 MHz</supported_graphics_clock>
                <supported_graphics_clock>615 MHz</supported_graphics_clock>
                <supported_graphics_clock>600 MHz</supported_graphics_clock>
                <supported_graphics_clock>585 MHz</supported_graphics_clock>
                <supported_graphics_clock>570 MHz</supported_graphics_clock>
                <supported_graphics_clock>555 MHz</supported_graphics_clock>
                <supported_graphics_clock>540 MHz</supported_graphics_clock>
                <supported_graphics_clock>525 MHz</supported_graphics_clock>
                <supported_graphics_clock>510 MHz</supported_graphics_clock>
                <supported_graphics_clock>495 MHz</supported_graphics_clock>
                <supported_graphics_clock>480 MHz</supported_graphics_clock>
                <supported_graphics_clock>465 MHz</supported_graphics_clock>
                <supported_graphics_clock>450 MHz</supported_graphics_clock>
                <supported_graphics_clock>435 MHz</supported_graphics_clock>
                <supported_graphics_clock>420 MHz</supported_graphics_clock>
                <supported_graphics_clock>405 MHz</supported_graphics_clock>
                <supported_graphics_clock>390 MHz</supported_graphics_clock>
                <supported_graphics_clock>375 MHz</supported_graphics_clock>
                <supported_graphics_clock>360 MHz</supported_graphics_clock>
                <supported_graphics_clock>345 MHz</supported_graphics_clock>
                <supported_graphics_clock>330 MHz</supported_graphics_clock>
                <supported_graphics_clock>315 MHz</supported_graphics_clock>
                <supported_graphics_clock>300 MHz</supported_graphics_clock>
                <supported_graphics_clock>285 MHz</supported_graphics_clock>
                <supported_graphics_clock>270 MHz</supported_graphics_clock>
                <supported_graphics_clock>255 MHz</supported_graphics_clock>
                <supported_graphics_clock>240 MHz</supported_graphics_clock>
                <supported_graphics_clock>225 MHz</supported_graphics_clock>
                <supported_graphics_clock>210 MHz</supported_graphics_clock>
            </supported_mem_clock>
            <supported_mem_clock>
                <value>405 MHz</value>
                <supported_graphics_clock>645 MHz</supported_graphics_clock>
                <supported_graphics_clock>630 MHz</supported_graphics_clock>
                <supported_graphics_clock>615 MHz</supported_graphics_clock>
                <supported_graphics_clock>600 MHz</supported_graphics_clock>
                <supported_graphics_clock>585 MHz</supported_graphics_clock>
                <supported_graphics_clock>570 MHz</supported_graphics_clock>
                <supported_graphics_clock>555 MHz</supported_graphics_clock>
                <supported_graphics_clock>540 MHz</supported_graphics_clock>
                <supported_graphics_clock>525 MHz</supported_graphics_clock>
                <supported_graphics_clock>510 MHz</supported_graphics_clock>
                <supported_graphics_clock>495 MHz</supported_graphics_clock>
                <supported_graphics_clock>480 MHz</supported_graphics_clock>
                <supported_graphics_clock>465 MHz</supported_graphics_clock>
                <supported_graphics_clock>450 MHz</supported_graphics_clock>
                <supported_graphics_clock>435 MHz</supported_graphics_clock>
                <supported_graphics_clock>420 MHz</supported_graphics_clock>
                <supported_graphics_clock>405 MHz</supported_graphics_clock>
                <supported_graphics_clock>390 MHz</supported_graphics_clock>
                <supported_graphics_clock>375 MHz</supported_graphics_clock>
                <supported_graphics_clock>360 MHz</supported_graphics_clock>
                <supported_graphics_clock>345 MHz</supported_graphics_clock>
                <supported_graphics_clock>330 MHz</supported_graphics_clock>
                <supported_graphics_clock>315 MHz</supported_graphics_clock>
                <supported_graphics_clock>300 MHz</supported_graphics_clock>
                <supported_graphics_clock>285 MHz</supported_graphics_clock>
                <supported_graphics_clock>270 MHz</supported_graphics_clock>
                <supported_graphics_clock>255 MHz</supported_graphics_clock>
                <supported_graphics_clock>240 MHz</supported_graphics_clock>
                <supported_graphics_clock>225 MHz</supported_graphics_clock>
                <supported_graphics_clock>210 MHz</supported_graphics_clock>
            </supported_mem_clock>
        </supported_clocks>
        <processes>
            <process_info>
                <pid>10131</pid>
                <type>C</type>
                <process_name>ffmpeg</process_name>
                <used_memory>389 MiB</used_memory>
            </process_info>
            <process_info>
                <pid>13597</pid>
                <type>C</type>
                <process_name>ffmpeg</process_name>
                <used_memory>1054 MiB</used_memory>
            </process_info>
        </processes>
        <accounted_processes>
        </accounted_processes>
        <capabilities>
            <egm>disabled</egm>
        </capabilities>
    </gpu>

    <gpu id="00000000:C1:00.0">
        <product_name>NVIDIA L4</product_name>
        <product_brand>NVIDIA</product_brand>
        <product_architecture>Ada Lovelace</product_architecture>
        <display_mode>Enabled</display_mode>
        <display_active>Disabled</display_active>
        <persistence_mode>Disabled</persistence_mode>
        <addressing_mode>None</addressing_mode>
        <mig_mode>
            <current_mig>N/A</current_mig>
            <pending_mig>N/A</pending_mig>
        </mig_mode>
        <mig_devices>
			None
        </mig_devices>
        <accounting_mode>Disabled</accounting_mode>
        <accounting_mode_buffer_size>4000</accounting_mode_buffer_size>
        <driver_model>
            <current_dm>N/A</current_dm>
            <pending_dm>N/A</pending_dm>
        </driver_model>
        <serial>1654523001128</serial>
        <uuid>GPU-128ab6fb-6ec9-fd74-b479-4a5fd14f55bd</uuid>
        <minor_number>0</minor_number>
        <vbios_version>95.04.29.00.06</vbios_version>
        <multigpu_board>No</multigpu_board>
        <board_id>0xc100</board_id>
        <board_part_number>900-2G193-0000-001</board_part_number>
        <gpu_part_number>27B8-895-A1</gpu_part_number>
        <gpu_fru_part_number>N/A</gpu_fru_part_number>
        <gpu_module_id>1</gpu_module_id>
        <inforom_version>
            <img_version>G193.0200.00.01</img_version>
            <oem_object>2.1</oem_object>
            <ecc_object>6.16</ecc_object>
            <pwr_object>N/A</pwr_object>
        </inforom_version>
        <inforom_bbx_flush>
            <latest_timestamp>N/A</latest_timestamp>
            <latest_duration>N/A</latest_duration>
        </inforom_bbx_flush>
        <gpu_operation_mode>
            <current_gom>N/A</current_gom>
            <pending_gom>N/A</pending_gom>
        </gpu_operation_mode>
        <c2c_mode>N/A</c2c_mode>
        <gpu_virtualization_mode>
            <virtualization_mode>None</virtualization_mode>
            <host_vgpu_mode>N/A</host_vgpu_mode>
            <vgpu_heterogeneous_mode>N/A</vgpu_heterogeneous_mode>
        </gpu_virtualization_mode>
        <gpu_reset_status>
            <reset_required>No</reset_required>
            <drain_and_reset_recommended>N/A</drain_and_reset_recommended>
        </gpu_reset_status>
        <gsp_firmware_version>555.42.06</gsp_firmware_version>
        <ibmnpu>
            <relaxed_ordering_mode>N/A</relaxed_ordering_mode>
        </ibmnpu>
        <pci>
            <pci_bus>C1</pci_bus>
            <pci_device>00</pci_device>
            <pci_domain>0000</pci_domain>
            <pci_base_class>3</pci_base_class>
            <pci_sub_class>2</pci_sub_class>
            <pci_device_id>27B810DE</pci_device_id>
            <pci_bus_id>00000000:C1:00.0</pci_bus_id>
            <pci_sub_system_id>16CA10DE</pci_sub_system_id>
            <pci_gpu_link_info>
                <pcie_gen>
                    <max_link_gen>4</max_link_gen>
                    <current_link_gen>4</current_link_gen>
                    <device_current_link_gen>4</device_current_link_gen>
                    <max_device_link_gen>4</max_device_link_gen>
                    <max_host_link_gen>5</max_host_link_gen>
                </pcie_gen>
                <link_widths>
                    <max_link_width>16x</max_link_width>
                    <current_link_width>1x</current_link_width>
                </link_widths>
            </pci_gpu_link_info>
            <pci_bridge_chip>
                <bridge_chip_type>N/A</bridge_chip_type>
                <bridge_chip_fw>N/A</bridge_chip_fw>
            </pci_bridge_chip>
            <replay_counter>0</replay_counter>
            <replay_rollover_counter>0</replay_rollover_counter>
            <tx_util>0 KB/s</tx_util>
            <rx_util>0 KB/s</rx_util>
            <atomic_caps_inbound>N/A</atomic_caps_inbound>
            <atomic_caps_outbound>N/A</atomic_caps_outbound>
        </pci>
        <fan_speed>N/A</fan_speed>
        <performance_state>P0</performance_state>
        <clocks_event_reasons>
            <clocks_event_reason_gpu_idle>Active</clocks_event_reason_gpu_idle>
            <clocks_event_reason_applications_clocks_setting>Not Active</clocks_event_reason_applications_clocks_setting>
            <clocks_event_reason_sw_power_cap>Not Active</clocks_event_reason_sw_power_cap>
            <clocks_event_reason_hw_slowdown>Not Active</clocks_event_reason_hw_slowdown>
            <clocks_event_reason_hw_thermal_slowdown>Not Active</clocks_event_reason_hw_thermal_slowdown>
            <clocks_event_reason_hw_power_brake_slowdown>Not Active</clocks_event_reason_hw_power_brake_slowdown>
            <clocks_event_reason_sync_boost>Not Active</clocks_event_reason_sync_boost>
            <clocks_event_reason_sw_thermal_slowdown>Not Active</clocks_event_reason_sw_thermal_slowdown>
            <clocks_event_reason_display_clocks_setting>Not Active</clocks_event_reason_display_clocks_setting>
        </clocks_event_reasons>
        <sparse_operation_mode>N/A</sparse_operation_mode>
        <fb_memory_usage>
            <total>23034 MiB</total>
            <reserved>434 MiB</reserved>
            <used>1 MiB</used>
            <free>22601 MiB</free>
        </fb_memory_usage>
        <bar1_memory_usage>
            <total>32768 MiB</total>
            <used>1 MiB</used>
            <free>32767 MiB</free>
        </bar1_memory_usage>
        <cc_protected_memory_usage>
            <total>0 MiB</total>
            <used>0 MiB</used>
            <free>0 MiB</free>
        </cc_protected_memory_usage>
        <compute_mode>Default</compute_mode>
        <utilization>
            <gpu_util>3 %</gpu_util>
            <memory_util>0 %</memory_util>
            <encoder_util>0 %</encoder_util>
            <decoder_util>0 %</decoder_util>
            <jpeg_util>0 %</jpeg_util>
            <ofa_util>0 %</ofa_util>
        </utilization>
        <encoder_stats>
            <session_count>0</session_count>
            <average_fps>0</average_fps>
            <average_latency>0</average_latency>
        </encoder_stats>
        <fbc_stats>
            <session_count>0</session_count>
            <average_fps>0</average_fps>
            <average_latency>0</average_latency>
        </fbc_stats>
        <ecc_mode>
            <current_ecc>Enabled</current_ecc>
            <pending_ecc>Enabled</pending_ecc>
        </ecc_mode>
        <ecc_errors>
            <volatile>
                <sram_correctable>0</sram_correctable>
                <sram_uncorrectable_parity>0</sram_uncorrectable_parity>
                <sram_uncorrectable_secded>0</sram_uncorrectable_secded>
                <dram_correctable>0</dram_correctable>
                <dram_uncorrectable>0</dram_uncorrectable>
            </volatile>
            <aggregate>
                <sram_correctable>0</sram_correctable>
                <sram_uncorrectable_parity>0</sram_uncorrectable_parity>
                <sram_uncorrectable_secded>0</sram_uncorrectable_secded>
                <dram_correctable>0</dram_correctable>
                <dram_uncorrectable>0</dram_uncorrectable>
                <sram_threshold_exceeded>No</sram_threshold_exceeded>
            </aggregate>
            <aggregate_uncorrectable_sram_sources>
                <sram_l2>0</sram_l2>
                <sram_sm>0</sram_sm>
                <sram_microcontroller>0</sram_microcontroller>
                <sram_pcie>0</sram_pcie>
                <sram_other>0</sram_other>
            </aggregate_uncorrectable_sram_sources>
        </ecc_errors>
        <retired_pages>
            <multiple_single_bit_retirement>
                <retired_count>N/A</retired_count>
                <retired_pagelist>N/A</retired_pagelist>
            </multiple_single_bit_retirement>
            <double_bit_retirement>
                <retired_count>N/A</retired_count>
                <retired_pagelist>N/A</retired_pagelist>
            </double_bit_retirement>
            <pending_blacklist>N/A</pending_blacklist>
            <pending_retirement>N/A</pending_retirement>
        </retired_pages>
        <remapped_rows>
            <remapped_row_corr>0</remapped_row_corr>
            <remapped_row_unc>0</remapped_row_unc>
            <remapped_row_pending>No</remapped_row_pending>
            <remapped_row_failure>No</remapped_row_failure>
            <row_remapper_histogram>
                <row_remapper_histogram_max>96 bank(s)</row_remapper_histogram_max>
                <row_remapper_histogram_high>0 bank(s)</row_remapper_histogram_high>
                <row_remapper_histogram_partial>0 bank(s)</row_remapper_histogram_partial>
                <row_remapper_histogram_low>0 bank(s)</row_remapper_histogram_low>
                <row_remapper_histogram_none>0 bank(s)</row_remapper_histogram_none>
            </row_remapper_histogram>
        </remapped_rows>
        <temperature>
            <gpu_temp>40 C</gpu_temp>
            <gpu_temp_tlimit>43 C</gpu_temp_tlimit>
            <gpu_temp_max_tlimit_threshold>-5 C</gpu_temp_max_tlimit_threshold>
            <gpu_temp_slow_tlimit_threshold>-2 C</gpu_temp_slow_tlimit_threshold>
            <gpu_temp_max_gpu_tlimit_threshold>0 C</gpu_temp_max_gpu_tlimit_threshold>
            <gpu_target_temperature>N/A</gpu_target_temperature>
            <memory_temp>N/A</memory_temp>
            <gpu_temp_max_mem_tlimit_threshold>N/A</gpu_temp_max_mem_tlimit_threshold>
        </temperature>
        <supported_gpu_target_temp>
            <gpu_target_temp_min>N/A</gpu_target_temp_min>
            <gpu_target_temp_max>N/A</gpu_target_temp_max>
        </supported_gpu_target_temp>
        <gpu_power_readings>
            <power_state>P0</power_state>
            <power_draw>29.54 W</power_draw>
            <current_power_limit>72.00 W</current_power_limit>
            <requested_power_limit>72.00 W</requested_power_limit>
            <default_power_limit>72.00 W</default_power_limit>
            <min_power_limit>40.00 W</min_power_limit>
            <max_power_limit>72.00 W</max_power_limit>
        </gpu_power_readings>
        <gpu_memory_power_readings>
            <power_draw>N/A</power_draw>
        </gpu_memory_power_readings>
        <module_power_readings>
            <power_state>P0</power_state>
            <power_draw>N/A</power_draw>
            <current_power_limit>N/A</current_power_limit>
            <requested_power_limit>N/A</requested_power_limit>
            <default_power_limit>N/A</default_power_limit>
            <min_power_limit>N/A</min_power_limit>
            <max_power_limit>N/A</max_power_limit>
        </module_power_readings>
        <clocks>
            <graphics_clock>2040 MHz</graphics_clock>
            <sm_clock>2040 MHz</sm_clock>
            <mem_clock>6250 MHz</mem_clock>
            <video_clock>1770 MHz</video_clock>
        </clocks>
        <applications_clocks>
            <graphics_clock>2040 MHz</graphics_clock>
            <mem_clock>6251 MHz</mem_clock>
        </applications_clocks>
        <default_applications_clocks>
            <graphics_clock>2040 MHz</graphics_clock>
            <mem_clock>6251 MHz</mem_clock>
        </default_applications_clocks>
        <deferred_clocks>
            <mem_clock>N/A</mem_clock>
        </deferred_clocks>
        <max_clocks>
            <graphics_clock>2040 MHz</graphics_clock>
            <sm_clock>2040 MHz</sm_clock>
            <mem_clock>6251 MHz</mem_clock>
            <video_clock>1770 MHz</video_clock>
        </max_clocks>
        <max_customer_boost_clocks>
            <graphics_clock>2040 MHz</graphics_clock>
        </max_customer_boost_clocks>
        <clock_policy>
            <auto_boost>N/A</auto_boost>
            <auto_boost_default>N/A</auto_boost_default>
        </clock_policy>
        <voltage>
            <graphics_volt>910.000 mV</graphics_volt>
        </voltage>
        <fabric>
            <state>N/A</state>
            <status>N/A</status>
            <cliqueId>N/A</cliqueId>
            <clusterUuid>N/A</clusterUuid>
            <health>
                <bandwidth>N/A</bandwidth>
            </health>
        </fabric>
        <supported_clocks>
            <supported_mem_clock>
                <value>6251 MHz</value>
                <supported_graphics_clock>2040 MHz</supported_graphics_clock>
                <supported_graphics_clock>2025 MHz</supported_graphics_clock>
                <supported_graphics_clock>2010 MHz</supported_graphics_clock>
                <supported_graphics_clock>1995 MHz</supported_graphics_clock>
                <supported_graphics_clock>1980 MHz</supported_graphics_clock>
                <supported_graphics_clock>1965 MHz</supported_graphics_clock>
                <supported_graphics_clock>1950 MHz</supported_graphics_clock>
                <supported_graphics_clock>1935 MHz</supported_graphics_clock>
                <supported_graphics_clock>1920 MHz</supported_graphics_clock>
                <supported_graphics_clock>1905 MHz</supported_graphics_clock>
                <supported_graphics_clock>1890 MHz</supported_graphics_clock>
                <supported_graphics_clock>1875 MHz</supported_graphics_clock>
                <supported_graphics_clock>1860 MHz</supported_graphics_clock>
                <supported_graphics_clock>1845 MHz</supported_graphics_clock>
                <supported_graphics_clock>1830 MHz</supported_graphics_clock>
                <supported_graphics_clock>1815 MHz</supported_graphics_clock>
                <supported_graphics_clock>1800 MHz</supported_graphics_clock>
                <supported_graphics_clock>1785 MHz</supported_graphics_clock>
                <supported_graphics_clock>1770 MHz</supported_graphics_clock>
                <supported_graphics_clock>1755 MHz</supported_graphics_clock>
                <supported_graphics_clock>1740 MHz</supported_graphics_clock>
                <supported_graphics_clock>1725 MHz</supported_graphics_clock>
                <supported_graphics_clock>1710 MHz</supported_graphics_clock>
                <supported_graphics_clock>1695 MHz</supported_graphics_clock>
                <supported_graphics_clock>1680 MHz</supported_graphics_clock>
                <supported_graphics_clock>1665 MHz</supported_graphics_clock>
                <supported_graphics_clock>1650 MHz</supported_graphics_clock>
                <supported_graphics_clock>1635 MHz</supported_graphics_clock>
                <supported_graphics_clock>1620 MHz</supported_graphics_clock>
                <supported_graphics_clock>1605 MHz</supported_graphics_clock>
                <supported_graphics_clock>1590 MHz</supported_graphics_clock>
                <supported_graphics_clock>1575 MHz</supported_graphics_clock>
                <supported_graphics_clock>1560 MHz</supported_graphics_clock>
                <supported_graphics_clock>1545 MHz</supported_graphics_clock>
                <supported_graphics_clock>1530 MHz</supported_graphics_clock>
                <supported_graphics_clock>1515 MHz</supported_graphics_clock>
                <supported_graphics_clock>1500 MHz</supported_graphics_clock>
                <supported_graphics_clock>1485 MHz</supported_graphics_clock>
                <supported_graphics_clock>1470 MHz</supported_graphics_clock>
                <supported_graphics_clock>1455 MHz</supported_graphics_clock>
                <supported_graphics_clock>1440 MHz</supported_graphics_clock>
                <supported_graphics_clock>1425 MHz</supported_graphics_clock>
                <supported_graphics_clock>1410 MHz</supported_graphics_clock>
                <supported_graphics_clock>1395 MHz</supported_graphics_clock>
                <supported_graphics_clock>1380 MHz</supported_graphics_clock>
                <supported_graphics_clock>1365 MHz</supported_graphics_clock>
                <supported_graphics_clock>1350 MHz</supported_graphics_clock>
                <supported_graphics_clock>1335 MHz</supported_graphics_clock>
                <supported_graphics_clock>1320 MHz</supported_graphics_clock>
                <supported_graphics_clock>1305 MHz</supported_graphics_clock>
                <supported_graphics_clock>1290 MHz</supported_graphics_clock>
                <supported_graphics_clock>1275 MHz</supported_graphics_clock>
                <supported_graphics_clock>1260 MHz</supported_graphics_clock>
                <supported_graphics_clock>1245 MHz</supported_graphics_clock>
                <supported_graphics_clock>1230 MHz</supported_graphics_clock>
                <supported_graphics_clock>1215 MHz</supported_graphics_clock>
                <supported_graphics_clock>1200 MHz</supported_graphics_clock>
                <supported_graphics_clock>1185 MHz</supported_graphics_clock>
                <supported_graphics_clock>1170 MHz</supported_graphics_clock>
                <supported_graphics_clock>1155 MHz</supported_graphics_clock>
                <supported_graphics_clock>1140 MHz</supported_graphics_clock>
                <supported_graphics_clock>1125 MHz</supported_graphics_clock>
                <supported_graphics_clock>1110 MHz</supported_graphics_clock>
                <supported_graphics_clock>1095 MHz</supported_graphics_clock>
                <supported_graphics_clock>1080 MHz</supported_graphics_clock>
                <supported_graphics_clock>1065 MHz</supported_graphics_clock>
                <supported_graphics_clock>1050 MHz</supported_graphics_clock>
                <supported_graphics_clock>1035 MHz</supported_graphics_clock>
                <supported_graphics_clock>1020 MHz</supported_graphics_clock>
                <supported_graphics_clock>1005 MHz</supported_graphics_clock>
                <supported_graphics_clock>990 MHz</supported_graphics_clock>
                <supported_graphics_clock>975 MHz</supported_graphics_clock>
                <supported_graphics_clock>960 MHz</supported_graphics_clock>
                <supported_graphics_clock>945 MHz</supported_graphics_clock>
                <supported_graphics_clock>930 MHz</supported_graphics_clock>
                <supported_graphics_clock>915 MHz</supported_graphics_clock>
                <supported_graphics_clock>900 MHz</supported_graphics_clock>
                <supported_graphics_clock>885 MHz</supported_graphics_clock>
                <supported_graphics_clock>870 MHz</supported_graphics_clock>
                <supported_graphics_clock>855 MHz</supported_graphics_clock>
                <supported_graphics_clock>840 MHz</supported_graphics_clock>
                <supported_graphics_clock>825 MHz</supported_graphics_clock>
                <supported_graphics_clock>810 MHz</supported_graphics_clock>
                <supported_graphics_clock>795 MHz</supported_graphics_clock>
                <supported_graphics_clock>780 MHz</supported_graphics_clock>
                <supported_graphics_clock>765 MHz</supported_graphics_clock>
                <supported_graphics_clock>750 MHz</supported_graphics_clock>
                <supported_graphics_clock>735 MHz</supported_graphics_clock>
                <supported_graphics_clock>720 MHz</supported_graphics_clock>
                <supported_graphics_clock>705 MHz</supported_graphics_clock>
                <supported_graphics_clock>690 MHz</supported_graphics_clock>
                <supported_graphics_clock>675 MHz</supported_graphics_clock>
                <supported_graphics_clock>660 MHz</supported_graphics_clock>
                <supported_graphics_clock>645 MHz</supported_graphics_clock>
                <supported_graphics_clock>630 MHz</supported_graphics_clock>
                <supported_graphics_clock>615 MHz</supported_graphics_clock>
                <supported_graphics_clock>600 MHz</supported_graphics_clock>
                <supported_graphics_clock>585 MHz</supported_graphics_clock>
                <supported_graphics_clock>570 MHz</supported_graphics_clock>
                <supported_graphics_clock>555 MHz</supported_graphics_clock>
                <supported_graphics_clock>540 MHz</supported_graphics_clock>
                <supported_graphics_clock>525 MHz</supported_graphics_clock>
                <supported_graphics_clock>510 MHz</supported_graphics_clock>
                <supported_graphics_clock>495 MHz</supported_graphics_clock>
                <supported_graphics_clock>480 MHz</supported_graphics_clock>
                <supported_graphics_clock>465 MHz</supported_graphics_clock>
                <supported_graphics_clock>450 MHz</supported_graphics_clock>
                <supported_graphics_clock>435 MHz</supported_graphics_clock>
                <supported_graphics_clock>420 MHz</supported_graphics_clock>
                <supported_graphics_clock>405 MHz</supported_graphics_clock>
                <supported_graphics_clock>390 MHz</supported_graphics_clock>
                <supported_graphics_clock>375 MHz</supported_graphics_clock>
                <supported_graphics_clock>360 MHz</supported_graphics_clock>
                <supported_graphics_clock>345 MHz</supported_graphics_clock>
                <supported_graphics_clock>330 MHz</supported_graphics_clock>
                <supported_graphics_clock>315 MHz</supported_graphics_clock>
                <supported_graphics_clock>300 MHz</supported_graphics_clock>
                <supported_graphics_clock>285 MHz</supported_graphics_clock>
                <supported_graphics_clock>270 MHz</supported_graphics_clock>
                <supported_graphics_clock>255 MHz</supported_graphics_clock>
                <supported_graphics_clock>240 MHz</supported_graphics_clock>
                <supported_graphics_clock>225 MHz</supported_graphics_clock>
                <supported_graphics_clock>210 MHz</supported_graphics_clock>
            </supported_mem_clock>
            <supported_mem_clock>
                <value>405 MHz</value>
                <supported_graphics_clock>645 MHz</supported_graphics_clock>
                <supported_graphics_clock>630 MHz</supported_graphics_clock>
                <supported_graphics_clock>615 MHz</supported_graphics_clock>
                <supported_graphics_clock>600 MHz</supported_graphics_clock>
                <supported_graphics_clock>585 MHz</supported_graphics_clock>
                <supported_graphics_clock>570 MHz</supported_graphics_clock>
                <supported_graphics_clock>555 MHz</supported_graphics_clock>
                <supported_graphics_clock>540 MHz</supported_graphics_clock>
                <supported_graphics_clock>525 MHz</supported_graphics_clock>
                <supported_graphics_clock>510 MHz</supported_graphics_clock>
                <supported_graphics_clock>495 MHz</supported_graphics_clock>
                <supported_graphics_clock>480 MHz</supported_graphics_clock>
                <supported_graphics_clock>465 MHz</supported_graphics_clock>
                <supported_graphics_clock>450 MHz</supported_graphics_clock>
                <supported_graphics_clock>435 MHz</supported_graphics_clock>
                <supported_graphics_clock>420 MHz</supported_graphics_clock>
                <supported_graphics_clock>405 MHz</supported_graphics_clock>
                <supported_graphics_clock>390 MHz</supported_graphics_clock>
                <supported_graphics_clock>375 MHz</supported_graphics_clock>
                <supported_graphics_clock>360 MHz</supported_graphics_clock>
                <supported_graphics_clock>345 MHz</supported_graphics_clock>
                <supported_graphics_clock>330 MHz</supported_graphics_clock>
                <supported_graphics_clock>315 MHz</supported_graphics_clock>
                <supported_graphics_clock>300 MHz</supported_graphics_clock>
                <supported_graphics_clock>285 MHz</supported_graphics_clock>
                <supported_graphics_clock>270 MHz</supported_graphics_clock>
                <supported_graphics_clock>255 MHz</supported_graphics_clock>
                <supported_graphics_clock>240 MHz</supported_graphics_clock>
                <supported_graphics_clock>225 MHz</supported_graphics_clock>
                <supported_graphics_clock>210 MHz</supported_graphics_clock>
            </supported_mem_clock>
        </supported_clocks>
        <processes>
            <process_info>
                <pid>16870</pid>
                <type>C</type>
                <process_name>ffmpeg</process_name>
                <used_memory>549 MiB</used_memory>
            </process_info>
        </processes>
        <accounted_processes>
        </accounted_processes>
        <capabilities>
            <egm>disabled</egm>
        </capabilities>
    </gpu>

</nvidia_smi_log>`

func main() {
	if len(os.Args) == 1 {
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	wait := false

	if os.Args[1] == "pmon" {
		if slices.Contains(os.Args[1:], "-c") {
			fmt.Fprintf(os.Stdout, "%s\n", pmondata)
		} else {
			go func(ctx context.Context) {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						fmt.Fprintf(os.Stdout, "%s\n", pmondata)
					}
				}
			}(ctx)
		}
	} else {
		if !slices.Contains(os.Args[1:], "-l") {
			fmt.Fprintf(os.Stdout, "%s\n", querydata)
		} else {
			wait = true
			go func(ctx context.Context) {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						fmt.Fprintf(os.Stdout, "%s\n", querydata)
					}
				}
			}(ctx)
		}
	}

	if wait {
		// Wait for interrupt signal to gracefully shutdown the app
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt)
		<-quit
	}

	cancel()

	os.Exit(0)
}

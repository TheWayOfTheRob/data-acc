# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License. You may obtain
# a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.

import os
import random

from burstbuffer.registry import api as registry

ASSIGNED_SLICES_KEY = "bufferhosts/assigned_slices/%s"
ALL_SLICES_KEY = "bufferhosts/all_slices/%s/%s"

FAKE_DEVICE_COUNT = 12
FAKE_DEVICE_ADDRESS = "nvme%sn1"
FAKE_DEVICE_SIZE_BYTES = int(1.5 * 2 ** 40)  # 1.5 TB


class UnexpectedBufferAssignement(Exception):
    pass


class UnableToAssignSlices(Exception):
    pass


def _get_local_hardware():
    fake_devices = []
    for i in range(FAKE_DEVICE_COUNT):
        fake_devices.append(FAKE_DEVICE_ADDRESS % i)
    return fake_devices


def _update_data(data, ensure_first_version=False):
    # TODO(johngarbutt) should be done in a single transaction
    for key, value in data.items():
        # TODO(johngarbutt) check version is 0 when ensure_first_version
        registry._etcdctl("put '%s' '%s'" % (key, value))


def _refresh_slices(hostname, hardware):
    slices_info = {}
    for device in hardware:
        key = ALL_SLICES_KEY % (hostname, device)
        slices_info[key] = FAKE_DEVICE_SIZE_BYTES
    _update_data(slices_info)


def _get_assigned_slices(hostname):
    prefix = ASSIGNED_SLICES_KEY % hostname
    raw_assignments = registry._get_all_with_prefix(prefix)
    current_devices = _get_local_hardware()

    assignments = {}
    for key in raw_assignments:
        device = key[(len(prefix) + 1):]
        if device not in current_devices:
            raise UnexpectedBufferAssignement(device)
        assignments[device] = raw_assignments[key]
    return assignments


def startup(hostname):
    all_slices = _get_local_hardware()
    _refresh_slices(hostname, all_slices)

    return _get_assigned_slices(hostname)


def _get_env():
    return os.environ


def _get_event_info():
    env = _get_env()

    event_type = env["ETCD_WATCH_EVENT_TYPE"]
    revision = env["ETCD_WATCH_REVISION"]
    key = env["ETCD_WATCH_KEY"]
    value = env['ETCD_WATCH_VALUE']

    return dict(
        event_type=event_type,
        revision=revision,
        key=key,
        value=value)


def event(hostname):
    event_info = _get_event_info()
    print(event_info)
    return _get_assigned_slices(hostname)


def _get_available_slices_by_host():
    raise NotImplementedError("TODO...")


def _set_assignments(buffer_id, assignments):
    assignments = list(assignments)
    # stop 0 always being the same host
    random.shuffle(assignments)
    data = {}

    for index in range(len(assignments)):
        host, device = assignments[index]

        # Add buffer to hosts slice assignments
        prefix = ASSIGNED_SLICES_KEY % host
        slice_key = "%s/%s" % (prefix, device)
        slice_value = "buffers/%s/slices/%s" % (buffer_id, index)
        data[slice_key] = slice_value

        # Add slice host to buffer
        buffer_key = slice_value
        data[buffer_key] = slice_key

    # TODO(johngarbutt) Add slices to buffer info
    for index in range(len(assignments)):
        host, device = assignments[index]

    # TODO(johngarbutt) ensure all updates were good in a transaction
    _update_data(data, ensure_first_version=True)


def assign_slices(buffer_id):
    buffer_info = registry.get_buffer(buffer_id)
    required_slices = buffer_info['capacity_slices']

    avaliable_slices_by_host = _get_available_slices_by_host()
    if len(avaliable_slices_by_host) < required_slices:
        raise UnableToAssignSlices("Not enough hosts for %s" % required_slices)

    assignments = set()
    for host, devices in avaliable_slices_by_host.items():
        # avoid some contention by not just picking the first
        device = random.choice(devices)
        assignments.add((host, device))

    _set_assignments(buffer_id, assignments)

    return assignments


def unassign_slices(buffer_id):
    raise NotImplementedError("TODO...")

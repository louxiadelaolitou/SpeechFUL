import 'dart:async';
import 'dart:convert';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_auth_passkey/hnuhole_auth_passkey.dart';

String encoded(int count, [int value = 7]) =>
    base64Url.encode(List.filled(count, value)).replaceAll('=', '');
Map<String, dynamic> getOptions() => {
  'challenge': encoded(32), 'rpId': 'auth.example.invalid',
  'timeout': 60000, 'userVerification': 'required',
};
Map<String, dynamic> createOptions() => {
  'challenge': encoded(32), 'timeout': 60000,
  'rp': {'id': 'auth.example.invalid', 'name': 'Hnuhole'},
  'user': {'id': encoded(32, 9), 'name': encoded(32, 9), 'displayName': 'Hnuhole account'},
  'pubKeyCredParams': [{'type': 'public-key', 'alg': -7}],
  'excludeCredentials': <dynamic>[],
  'authenticatorSelection': {'residentKey': 'required', 'requireResidentKey': true, 'userVerification': 'required'},
  'attestation': 'none',
};
Map<String, dynamic> credential({bool create = false}) => {
  'id': encoded(16), 'rawId': encoded(16), 'type': 'public-key',
  'response': create ? <String, dynamic>{'clientDataJSON': encoded(40), 'attestationObject': encoded(100)} :
      <String, dynamic>{'clientDataJSON': encoded(40), 'authenticatorData': encoded(37), 'signature': encoded(72), 'userHandle': encoded(32, 9)},
  'clientExtensionResults': <String, dynamic>{},
};
Matcher fails(String code) => throwsA(isA<PasskeyFailure>().having((value) => value.code, 'code', code));

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const channel = MethodChannel('hnuhole/auth_passkey_test');
  late NativePasskeyClient client;
  void mock(Future<Object?> Function(MethodCall)? handler) {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(channel,
      handler == null ? null : (call) async => call.method == 'available' ? true : handler(call));
  }
  setUp(() { client = NativePasskeyClient(channel: channel, platform: TargetPlatform.android); });
  tearDown(() { mock(null); client.dispose(); });

  test('create passes fixed server policy and preserves signed fields', () async {
    final options = createOptions();
    mock((call) async {
      expect(call.method, 'create');
      expect(jsonDecode((call.arguments as Map)['publicKey'] as String), options);
      return jsonEncode(credential(create: true));
    });
    expect(await client.create(options), credential(create: true));
  });
  test('get discovers a credential without a username or allow list', () async {
    mock((call) async {
      final options = jsonDecode((call.arguments as Map)['publicKey'] as String) as Map;
      expect(options.containsKey('allowCredentials'), isFalse);
      expect(options.containsKey('user'), isFalse);
      return jsonEncode(credential());
    });
    expect(await client.get(getOptions()), credential());
  });
  test('unsafe creation policy fails before native work starts', () async {
    var calls = 0;
    mock((_) async { ++calls; return null; });
    final mutations = <void Function(Map<String, dynamic>)>[
      (value) => value['origin'] = 'https://auth.example.invalid',
      (value) => value['attestation'] = 'direct',
      (value) => (value['authenticatorSelection'] as Map)['userVerification'] = 'preferred',
      (value) => (value['authenticatorSelection'] as Map)['residentKey'] = 'preferred',
      (value) => (value['user'] as Map)['name'] = 'private_username',
      (value) => (value['pubKeyCredParams'] as List).add({'type': 'public-key', 'alg': -257}),
      (value) => value['challenge'] = '${encoded(32)}=',
      (value) => (value['rp'] as Map)['id'] = '127.0.0.1',
      (value) => (value['rp'] as Map)['id'] = 'auth.example.invalid\n',
    ];
    for (final mutate in mutations) {
      final options = createOptions(); mutate(options);
      await expectLater(client.create(options), fails('PASSKEY_INVALID_OPTIONS'));
    }
    expect(calls, 0);
  });
  test('recovery refuses a caller-provided allow list', () async {
    await expectLater(client.get({...getOptions(), 'allowCredentials': []}), fails('PASSKEY_INVALID_OPTIONS'));
  });
  test('invalid platform responses never reach callers', () async {
    final mutations = <void Function(Map<String, dynamic>)>[
      (value) => value['rawId'] = encoded(16, 8),
      (value) => value['authenticatorAttachment'] = 'platform',
      (value) => value['clientExtensionResults'] = {'appid': true},
      (value) => (value['response'] as Map)['userHandle'] = null,
      (value) => (value['response'] as Map)['userHandle'] = encoded(31),
      (value) => (value['response'] as Map)['authenticatorData'] = encoded(36),
      (value) => (value['response'] as Map)['clientDataJSON'] = encoded(3073),
    ];
    for (final mutate in mutations) {
      final response = credential(); mutate(response);
      mock((_) async => jsonEncode(response));
      await expectLater(client.get(getOptions()), fails('PASSKEY_INVALID_RESPONSE'));
    }
  });
  test('cancel suppresses late success and does not expose its credential', () async {
    final completion = Completer<String>();
    final started = Completer<void>();
    mock((call) async {
      if (call.method == 'cancel') return null;
      started.complete();
      return completion.future;
    });
    final first = client.get(getOptions());
    final assertion = expectLater(first, fails('PASSKEY_CANCELLED'));
    await started.future;
    await client.cancel();
    completion.complete(jsonEncode(credential()));
    await assertion;
  });
  test('a second operation is busy until the first has settled', () async {
    final completion = Completer<String>();
    mock((_) => completion.future);
    final first = client.get(getOptions());
    await expectLater(client.create(createOptions()), fails('PASSKEY_BUSY'));
    completion.complete(jsonEncode(credential()));
    await first;
  });
  test('known cancellation is retained and provider details are discarded', () async {
    mock((_) async {
      throw PlatformException(code: 'PASSKEY_CANCELLED', message: 'synthetic private data', details: 'synthetic assertion');
    });
    await expectLater(client.get(getOptions()), fails('PASSKEY_CANCELLED'));
    mock((_) async {
      throw PlatformException(code: 'PROVIDER_SECRET', details: 'synthetic private data');
    });
    await expectLater(client.get(getOptions()), fails('PASSKEY_FAILED'));
  });
  test('a missing plugin and unsupported platform fail closed', () async {
    mock(null);
    await expectLater(client.get(getOptions()), fails('PASSKEY_UNAVAILABLE'));
    await expectLater(NativePasskeyClient(channel: channel, platform: TargetPlatform.linux).get(getOptions()), fails('PASSKEY_UNAVAILABLE'));
  });

  test('provider removed after preflight invalidates the optional UI capability', () async {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(channel, (call) async {
      if (call.method == 'available') return true;
      throw PlatformException(code: 'PASSKEY_UNAVAILABLE');
    });
    await expectLater(client.get(getOptions()), fails('PASSKEY_UNAVAILABLE'));
    expect(client.passkeyAvailable, isFalse);
  });
  test(
    'unavailable device never opens a native ceremony or asks for another app',
    () async {
      final calls = <String>[];
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, (call) async {
            calls.add(call.method);
            return false;
          });
      expect(client.passkeyAvailable, isFalse);
      await expectLater(
        client.create(createOptions()),
        fails('PASSKEY_UNAVAILABLE'),
      );
      await expectLater(client.get(getOptions()), fails('PASSKEY_UNAVAILABLE'));
      expect(calls, ['available', 'available']);
    },
  );

  test(
    'capability refresh follows system changes and failed queries fail closed',
    () async {
      var supported = true;
      var failed = false;
      var changes = 0;
      client.addListener(() => changes++);
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, (call) async {
            expect(call.method, 'available');
            if (failed) throw PlatformException(code: 'unsupported');
            return supported;
          });
      expect(await client.refreshAvailability(), isTrue);
      expect(client.passkeyAvailable, isTrue);
      supported = false;
      expect(await client.refreshAvailability(), isFalse);
      supported = true;
      await client.refreshAvailability();
      failed = true;
      expect(await client.refreshAvailability(), isFalse);
      expect(changes, 4);
    },
  );

  test(
    'cancelling while capability query is pending cannot open a system sheet',
    () async {
      final availability = Completer<bool>();
      final calls = <String>[];
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, (call) async {
            calls.add(call.method);
            if (call.method == 'available') return availability.future;
            return null;
          });
      final operation = expectLater(
        client.get(getOptions()),
        fails('PASSKEY_CANCELLED'),
      );
      await client.cancel();
      availability.complete(true);
      await operation;
      expect(calls, ['available', 'cancel']);
    },
  );
}

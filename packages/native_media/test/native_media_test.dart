import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_native_media/hnuhole_native_media.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const channel = MethodChannel('speechful/test_media');
  const media = IosNativeMedia(channel: channel);
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  tearDown(() => messenger.setMockMethodCallHandler(channel, null));

  test(
    'selection keeps order, reads original bytes once and releases',
    () async {
      final calls = <MethodCall>[];
      final bytes = Uint8List.fromList([71, 73, 70, 56, 57, 97]);
      messenger.setMockMethodCallHandler(channel, (call) async {
        calls.add(call);
        return switch (call.method) {
          'pick' => [
            {'id': 'first', 'length': 6},
            {'id': 'second', 'length': 6},
          ],
          'read' => bytes,
          _ => null,
        };
      });
      final files = await media.pick(2);
      expect(calls.single.arguments, {'limit': 2});
      expect(await files.first.read(), bytes);
      expect(calls[1].arguments, 'first');
      expect(calls[2].method, 'release');
      await expectLater(files.first.read(), throwsStateError);
      await files.first.release();
      expect(calls.length, 3);
      await files.last.release();
      expect(calls.last.arguments, 'second');
    },
  );

  test('cancel is empty, limits fail before invoking native UI', () async {
    var calls = 0;
    messenger.setMockMethodCallHandler(channel, (call) async {
      calls++;
      return [];
    });
    await expectLater(media.pick(0), throwsRangeError);
    await expectLater(media.pick(10), throwsRangeError);
    expect(calls, 0);
    expect(await media.pick(1), isEmpty);
  });

  test('failed or changed file releases its opaque selection', () async {
    final released = <Object?>[];
    messenger.setMockMethodCallHandler(channel, (call) async {
      if (call.method == 'pick')
        return [
          {'id': 'one', 'length': 2},
        ];
      if (call.method == 'read') return Uint8List.fromList([1]);
      if (call.method == 'release') released.add(call.arguments);
      return null;
    });
    final file = (await media.pick(1)).single;
    await expectLater(file.read(), throwsFormatException);
    expect(released, ['one']);
  });

  test('native failures preserve actionable gallery error codes', () async {
    final calls = <String>[];
    messenger.setMockMethodCallHandler(channel, (call) async {
      calls.add(call.method);
      if (call.method == 'putImage')
        throw PlatformException(code: 'MEDIA_UNSUPPORTED');
      return call.method == 'requestAccess';
    });
    expect(await media.hasAccess(), isFalse);
    expect(await media.requestAccess(), isTrue);
    await expectLater(
      media.putImage('/original.webp'),
      throwsA(
        isA<PlatformException>().having(
          (e) => e.code,
          'code',
          'MEDIA_UNSUPPORTED',
        ),
      ),
    );
    expect(calls, ['hasAccess', 'requestAccess', 'putImage']);
  });
}

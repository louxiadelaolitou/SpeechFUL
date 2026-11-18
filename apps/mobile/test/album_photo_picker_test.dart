import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/src/media/album_photo_picker.dart';
import 'package:hnuhole_mobile/src/posts/post_photo_picker.dart';
import 'package:photo_manager/photo_manager.dart';

const _album = MethodChannel('com.fluttercandies/photo_manager');
const _start = ValueKey('open-album');

class _SystemPicker implements PostPhotoPicker {
  int? limit;

  @override
  Future<void> discardUnboundLostSelection() async {}

  @override
  Future<List<PostPickedFile>> pick(int limit) async {
    this.limit = limit;
    return [];
  }
}

void main() {
  testWidgets(
    'limited photo permission opens the album and caps multi-select',
    (tester) async {
      final messenger =
          TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
      messenger.setMockMethodCallHandler(_album, (call) async {
        switch (call.method) {
          case 'requestPermissionExtend':
            final options = call.arguments as Map;
            expect(
              options['androidPermission']['type'],
              RequestType.image.value,
            );
            expect(options['androidPermission']['mediaLocation'], isFalse);
            return PermissionState.limited.index;
          case 'getAssetPathList':
            return {
              'data': [
                {'id': 'all', 'name': 'Recent', 'isAll': true, 'assetCount': 3},
              ],
            };
          case 'getAssetCountFromPath':
            return 3;
          case 'getAssetListPaged':
            return {
              'data': [
                for (final id in ['a', 'b', 'c'])
                  {
                    'id': id,
                    'type': AssetType.image.index,
                    'width': 320,
                    'height': 240,
                  },
              ],
            };
          case 'getThumb':
            return null;
          default:
            throw MissingPluginException(call.method);
        }
      });
      addTearDown(() => messenger.setMockMethodCallHandler(_album, null));
      List<PostPickedFile>? selected;
      final system = _SystemPicker();
      await tester.pumpWidget(
        MaterialApp(
          home: Builder(
            builder: (context) {
              return TextButton(
                key: _start,
                onPressed: () async {
                  selected = await AlbumPhotoPicker(
                    context,
                    systemPicker: system,
                  ).pick(2);
                },
                child: const Text('Open'),
              );
            },
          ),
        ),
      );
      await tester.tap(find.byKey(_start));
      await tester.pumpAndSettle();
      for (final id in ['a', 'b', 'c']) {
        await tester.tap(find.byKey(ValueKey('album-photo-$id')));
        await tester.pump();
      }
      expect(find.text('2 / 2'), findsOneWidget);
      await tester.tap(find.byKey(const ValueKey('album-confirm-selection')));
      await tester.pumpAndSettle();
      expect(selected, hasLength(2));
      expect(system.limit, isNull);
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets(
    'denied album permission forwards the entry limit to system picker',
    (tester) async {
      final messenger =
          TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
      messenger.setMockMethodCallHandler(
        _album,
        (_) async => PermissionState.denied.index,
      );
      addTearDown(() => messenger.setMockMethodCallHandler(_album, null));
      final system = _SystemPicker();
      await tester.pumpWidget(
        MaterialApp(
          home: Builder(
            builder: (context) {
              return TextButton(
                key: _start,
                onPressed: () async {
                  await AlbumPhotoPicker(context, systemPicker: system).pick(3);
                },
                child: const Text('Open'),
              );
            },
          ),
        ),
      );
      await tester.tap(find.byKey(_start));
      await tester.pumpAndSettle();
      expect(system.limit, 3);
      expect(
        find.byKey(const ValueKey('album-confirm-selection')),
        findsNothing,
      );
    },
  );
}

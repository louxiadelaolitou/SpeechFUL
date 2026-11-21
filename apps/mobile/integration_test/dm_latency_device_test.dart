import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:hnuhole_mobile/src/messaging/dm_controller.dart';
import 'package:hnuhole_mobile/src/messaging/dm_store.dart';
import 'package:hnuhole_mobile/src/posts/http_post_api.dart';

String? dmTextOf(Map<String, dynamic> row) {
  final direct = row['text'];
  if (direct is String && direct.isNotEmpty) return direct;
  final intent = row['intent'];
  if (intent is Map) {
    final value = intent['text'];
    if (value is String) return value;
  }
  return null;
}

Future<Map<String, dynamic>> waitForDmState(
  DmController controller,
  String own,
  String peer,
  String text,
  Set<String> states, {
  Duration timeout = const Duration(seconds: 15),
}) async {
  final deadline = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(deadline)) {
    final rows = await controller.history(own, peer);
    final match = rows.where((row) => dmTextOf(row) == text).toList();
    if (match.length == 1 && states.contains(match.single['_state'])) {
      return match.single;
    }
    await Future<void>.delayed(const Duration(milliseconds: 10));
  }
  throw TestFailure(
    'DM message "$text" did not reach ${states.join('/')} within ${timeout.inSeconds}s',
  );
}

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  testWidgets('DM send latency on a real Android device', (tester) async {
    const origin = String.fromEnvironment('DM_ORIGIN');
    const ca = String.fromEnvironment('DM_CA');
    const a = String.fromEnvironment('DM_TOKEN_A'),
        b = String.fromEnvironment('DM_TOKEN_B');
    const aa = String.fromEnvironment('DM_ACCOUNT_A'),
        ba = String.fromEnvironment('DM_ACCOUNT_B');
    const ia = String.fromEnvironment('DM_IDENTITY_A'),
        ib = String.fromEnvironment('DM_IDENTITY_B');
    const post = String.fromEnvironment('DM_POST');
    expect(origin, isNotEmpty);
    final trust = SecurityContext(withTrustedRoots: false)
      ..setTrustedCertificatesBytes(base64Decode(ca));
    final api = HttpPostApi(
      communityBaseUri: Uri.parse(origin),
      client: HttpClient(context: trust),
    );
    Future<DmStore> open(String account) =>
        DmStore.openNative('dm-latency-device:$origin', account);
    var sender = DmController(api: api, openStore: open);
    await tester.runAsync(() async {
      await sender.bind(
        account: aa,
        token: a,
        identities: [ia],
        isCurrent: () => true,
      );
      final contact = await sender.contact(post, '');
      expect(contact['peerId'], ib);

      final runTag = DateTime.now().microsecondsSinceEpoch;
      final firstText = '延迟首条$runTag';
      final firstSend = Stopwatch()..start();
      expect(
        await sender.send({
          'identityId': ia,
          'peerId': ib,
          'conversationId': '',
          'sourcePostId': post,
          'sourceCommentId': '',
          'text': firstText,
          'attempt': 1,
        }),
        isTrue,
      );
      Map<String, dynamic>? first;
      for (var i = 0; i < 200; i++) {
        final rows = await sender.history(ia, ib);
        final match = rows.where((row) => dmTextOf(row) == firstText).toList();
        if (i % 10 == 0) {
          // ignore: avoid_print
          print(
            'DM_LATENCY_POLL t=${firstSend.elapsedMilliseconds} rows=${rows.length} '
            'match=${match.length} states=${match.map((row) => row['_state']).toList()}',
          );
        }
        if (match.length == 1 &&
            {'unknown', 'sent'}.contains(match.single['_state'])) {
          first = match.single;
          break;
        }
        await Future<void>.delayed(const Duration(milliseconds: 100));
      }
      firstSend.stop();
      // ignore: avoid_print
      print(
        'DM_SEND_LATENCY first_state=${first?['_state']} '
        'first_ms=${firstSend.elapsedMilliseconds}',
      );
      expect(first, isNotNull);
      final firstRow = first!;
      String conv;
      if (firstRow['_state'] == 'sent') {
        conv = (firstRow['receipt'] as Map)['conversationId'] as String;
      } else {
        await sender.detach();
        sender.dispose();
        sender = DmController(api: api, openStore: open);
        await sender.bind(
          account: aa,
          token: a,
          identities: [ia],
          isCurrent: () => true,
        );
        final recovered = await waitForDmState(sender, ia, ib, firstText, {
          'sent',
        });
        conv = (recovered['receipt'] as Map)['conversationId'] as String;
      }

      final receiver = DmController(api: api, openStore: open);
      await receiver.bind(
        account: ba,
        token: b,
        identities: [ib],
        isCurrent: () => true,
      );
      final incoming = await waitForDmState(receiver, ib, ia, firstText, {
        'inbox',
      });
      await receiver.visible([incoming['_id']]);
      await receiver.sync();

      final replyText = '延迟回复$runTag';
      final replySend = Stopwatch()..start();
      expect(
        await receiver.send({
          'identityId': ib,
          'peerId': ia,
          'conversationId': conv,
          'sourcePostId': '',
          'sourceCommentId': '',
          'text': replyText,
          'attempt': 1,
        }),
        isTrue,
      );
      await waitForDmState(receiver, ib, ia, replyText, {'sent'});
      replySend.stop();
      // ignore: avoid_print
      print('DM_SEND_LATENCY reply_sent_ms=${replySend.elapsedMilliseconds}');

      for (var i = 0; i < 5; i++) {
        final text = '时延样本$runTag-$i';
        final sample = Stopwatch()..start();
        expect(
          await receiver.send({
            'identityId': ib,
            'peerId': ia,
            'conversationId': conv,
            'sourcePostId': '',
            'sourceCommentId': '',
            'text': text,
            'attempt': 1,
          }),
          isTrue,
        );
        await waitForDmState(receiver, ib, ia, text, {'sent'});
        sample.stop();
        // ignore: avoid_print
        print('DM_SEND_LATENCY sample_${i}_ms=${sample.elapsedMilliseconds}');
      }
      await receiver.detach();
      receiver.dispose();
      await sender.detach();
      sender.dispose();
      api.close();
    });
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(body: Center(child: Text('DM 发送时延采集完成'))),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('DM 发送时延采集完成'), findsOneWidget);
  });
}

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
  Duration timeout = const Duration(seconds: 10),
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
  testWidgets('DM P1 real Android storage and real HTTPS text recovery', (
    tester,
  ) async {
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
    // Separate noBackup files and native Keystore namespaces for synthetic accounts.
    Future<DmStore> open(String account) =>
        DmStore.openNative('dm-p1-device:$origin', account);
    var controller = DmController(api: api, openStore: open);
    await tester.runAsync(() async {
      await controller.bind(
        account: aa,
        token: a,
        identities: [ia],
        isCurrent: () => true,
      );
      final contact = await controller.contact(post, '');
      expect(contact['peerId'], ib);
      final runTag = DateTime.now().microsecondsSinceEpoch;
      final firstText = '你好，真机可靠文字$runTag';
      final replyText = '收到，离线回复$runTag';
      await controller.saveDraft(ia, ib, firstText);
      final firstSend = Stopwatch()..start();
      expect(
        await controller.send({
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
      // The send lane must dispatch without an explicit sync(); the dropped
      // response leaves the row unknown after the server committed it.
      final first = await waitForDmState(controller, ia, ib, firstText, {
        'unknown',
      });
      firstSend.stop();
      // ignore: avoid_print
      print(
        'DM_SEND_LATENCY first_unknown_ms=${firstSend.elapsedMilliseconds}',
      );
      expect(first['_state'], 'unknown');
      await controller.sync();
      expect((await controller.history(ia, ib)).single['_state'], 'unknown');
      await controller.detach();
      controller.dispose();
      controller = DmController(api: api, openStore: open);
      await controller.bind(
        account: aa,
        token: a,
        identities: [ia],
        isCurrent: () => true,
      );
      final sent = (await controller.history(ia, ib)).single;
      expect(sent['_state'], 'sent');
      expect(await controller.draft(ia, ib), '');
      final conv = (sent['receipt'] as Map)['conversationId'] as String;
      await controller.detach();
      controller.dispose();
      final receiver = DmController(api: api, openStore: open);
      await receiver.bind(
        account: ba,
        token: b,
        identities: [ib],
        isCurrent: () => true,
      );
      final incoming = (await receiver.history(ib, ia)).single;
      expect(incoming['text'], firstText);
      expect(incoming['_state'], 'inbox');
      expect(
        (await api.dmRequest(b, 'GET', 'pending')).value['items'],
        isEmpty,
      );
      final unread = (await api.dmRequest(
        b,
        'GET',
        'conversations',
        query: {'identityId': ib},
      )).value;
      expect((unread['items'] as List).single['unread'], 1);
      await receiver.visible([incoming['_id']]);
      await receiver.sync();
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
      // The reply is a normal send: the immediate lane must reach sent without
      // waiting for the next sync() round.
      final reply = await waitForDmState(receiver, ib, ia, replyText, {'sent'});
      replySend.stop();
      // ignore: avoid_print
      print('DM_SEND_LATENCY reply_sent_ms=${replySend.elapsedMilliseconds}');
      expect(reply['_state'], 'sent');
      await receiver.sync();
      await receiver.detach();
      receiver.dispose();
      controller = DmController(api: api, openStore: open);
      await controller.bind(
        account: aa,
        token: a,
        identities: [ia],
        isCurrent: () => true,
      );
      final history = await controller.history(ia, ib);
      expect(history.length, 2);
      expect(history.last['text'], replyText);
      await controller.saveDraft(ia, ib, '重开后保留的草稿');
      await controller.detach();
      controller.dispose();
      final reopened = await open(aa);
      expect(await reopened.draft(ia, ib), '重开后保留的草稿');
      expect((await reopened.history(ia, ib)).length, 2);
      await reopened.close();
      api.close();
    });
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(body: Center(child: Text('可靠文字收发 · 真机验收通过'))),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('可靠文字收发 · 真机验收通过'), findsOneWidget);
  });
}
